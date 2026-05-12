package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	objectPathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	timestampStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	sizeStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	headerPathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	metadataTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	metadataKeyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("177"))
	previewTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	binaryNoticeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
)

type viewMode int

const (
	viewBuckets viewMode = iota
	viewObjects
	viewDetail
)

type model struct {
	service s3Service

	mode    viewMode
	loading bool
	err     error

	width  int
	height int

	buckets      []bucketItem
	bucketCursor int
	bucketScroll int
	activeBucket string

	objects      []objectItem
	root         *treeNode
	current      *treeNode
	pathStack    []*treeNode
	objectCursor int
	objectScroll int

	detail       objectDetail
	detailScroll int
}

type bucketsLoadedMsg struct {
	buckets []bucketItem
	err     error
}

type objectsLoadedMsg struct {
	bucket  string
	objects []objectItem
	err     error
}

type detailLoadedMsg struct {
	detail objectDetail
	err    error
}

func newModel(service s3Service) model {
	return model{service: service, mode: viewBuckets, loading: true}
}

func (m model) Init() tea.Cmd {
	return m.loadBuckets()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case bucketsLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.buckets = msg.buckets
		m.bucketCursor = clampCursor(m.bucketCursor, len(m.buckets))
		m.bucketScroll = clampListScroll(m.bucketScroll, m.bucketCursor, len(m.buckets), visibleHeight(m.height))
	case objectsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.mode = viewObjects
			m.activeBucket = msg.bucket
			m.objects = msg.objects
			m.root = buildObjectTree(msg.objects)
			m.current = m.root
			m.pathStack = nil
			m.objectCursor = 0
			m.objectScroll = 0
		}
	case detailLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.mode = viewDetail
			m.detail = msg.detail
			m.detailScroll = 0
		}
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "r":
		m.loading = true
		m.err = nil
		switch m.mode {
		case viewBuckets:
			return m, m.loadBuckets()
		case viewObjects:
			return m, m.loadObjects(m.activeBucket)
		case viewDetail:
			return m, m.loadDetail(m.activeBucket, m.detail.Object.Key)
		}
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-10)
	case "pgdown":
		m.moveCursor(10)
	case "enter":
		return m.activateSelection()
	case "backspace", "esc", "left", "h":
		return m.goBack()
	}
	return m, nil
}

func (m *model) moveCursor(delta int) {
	switch m.mode {
	case viewBuckets:
		m.bucketCursor = wrapCursor(m.bucketCursor, delta, len(m.buckets))
		m.bucketScroll = clampListScroll(m.bucketScroll, m.bucketCursor, len(m.buckets), visibleHeight(m.height))
	case viewObjects:
		entries := listChildren(m.current)
		m.objectCursor = wrapCursor(m.objectCursor, delta, len(entries))
		m.objectScroll = clampListScroll(m.objectScroll, m.objectCursor, len(entries), visibleHeight(m.height))
	case viewDetail:
		m.detailScroll = clamp(m.detailScroll+delta, 0, m.maxDetailScroll())
	}
}

func (m model) activateSelection() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	switch m.mode {
	case viewBuckets:
		if len(m.buckets) == 0 {
			return m, nil
		}
		bucket := m.buckets[m.bucketCursor].Name
		m.loading = true
		m.err = nil
		return m, m.loadObjects(bucket)
	case viewObjects:
		entries := listChildren(m.current)
		if len(entries) == 0 {
			return m, nil
		}
		node := entries[m.objectCursor].Node
		if node.Kind == nodeFolder {
			m.pathStack = append(m.pathStack, m.current)
			m.current = node
			m.objectCursor = 0
			m.objectScroll = 0
			return m, nil
		}
		m.loading = true
		m.err = nil
		return m, m.loadDetail(m.activeBucket, node.Path)
	}
	return m, nil
}

func (m model) goBack() (tea.Model, tea.Cmd) {
	switch m.mode {
	case viewDetail:
		m.mode = viewObjects
		m.err = nil
	case viewObjects:
		if len(m.pathStack) > 0 {
			m.current = m.pathStack[len(m.pathStack)-1]
			m.pathStack = m.pathStack[:len(m.pathStack)-1]
			m.objectCursor = 0
			m.objectScroll = 0
		} else {
			m.mode = viewBuckets
			m.err = nil
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	if m.loading {
		b.WriteString("Loading...\n")
	}
	if m.err != nil {
		b.WriteString("Error: " + m.err.Error() + "\n\n")
	}

	switch m.mode {
	case viewBuckets:
		b.WriteString(m.viewBuckets())
	case viewObjects:
		b.WriteString(m.viewObjects())
	case viewDetail:
		b.WriteString(m.viewDetail())
	}
	b.WriteString("\n")
	b.WriteString("enter open/view  backspace back  r reload  q quit")
	return b.String()
}

func (m model) header() string {
	switch m.mode {
	case viewBuckets:
		return "S3 Objects Browser - buckets"
	case viewObjects:
		return fmt.Sprintf("S3 Objects Browser - %s:%s", m.activeBucket, currentPath(m.current))
	case viewDetail:
		return "S3 Objects Browser - " + headerPathStyle.Render(fmt.Sprintf("%s/%s", m.activeBucket, m.detail.Object.Key))
	default:
		return "S3 Objects Browser"
	}
}

const listTimestampLayout = "2006-01-02 15:04:05"

func (m model) viewBuckets() string {
	if len(m.buckets) == 0 && !m.loading {
		return "No buckets found.\n"
	}
	var b strings.Builder
	visible := visibleHeight(m.height)
	scroll := clampListScroll(m.bucketScroll, m.bucketCursor, len(m.buckets), visible)
	end := min(len(m.buckets), scroll+visible)
	for i := scroll; i < end; i++ {
		bucket := m.buckets[i]
		cursor := " "
		if i == m.bucketCursor {
			cursor = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s %s\n", cursor, styledListTimestamp(bucket.CreationDate), bucket.Name))
	}
	return b.String()
}

func (m model) viewObjects() string {
	entries := listChildren(m.current)
	if len(entries) == 0 && !m.loading {
		return "No objects found.\n"
	}
	var b strings.Builder
	visible := visibleHeight(m.height)
	scroll := clampListScroll(m.objectScroll, m.objectCursor, len(entries), visible)
	end := min(len(entries), scroll+visible)
	for i := scroll; i < end; i++ {
		entry := entries[i]
		cursor := " "
		if i == m.objectCursor {
			cursor = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s %s %s\n", cursor, styledObjectTimestamp(entry.Node), styledObjectSize(entry.Node), entry.Label))
	}
	return b.String()
}

func styledListTimestamp(value time.Time) string {
	return timestampStyle.Render(formatListTimestamp(value))
}

func styledObjectTimestamp(node *treeNode) string {
	if node != nil && node.Kind == nodeFolder {
		return timestampStyle.Render(fmt.Sprintf("%-*s", len(listTimestampLayout), "PREFIX"))
	}
	if node == nil || node.Object == nil {
		return timestampStyle.Render(blankTimestamp())
	}
	return styledListTimestamp(node.Object.LastModified)
}

func styledObjectSize(node *treeNode) string {
	if node == nil || node.Kind != nodeObject || node.Object == nil {
		return sizeStyle.Render(blankObjectSize())
	}
	return sizeStyle.Render(fmt.Sprintf("%12s", formatBytes(node.Object.Size)))
}

func formatListTimestamp(value time.Time) string {
	if value.IsZero() {
		return blankTimestamp()
	}
	return value.Format(listTimestampLayout)
}

func blankTimestamp() string {
	return strings.Repeat(" ", len(listTimestampLayout))
}

func blankObjectSize() string {
	return strings.Repeat(" ", 12)
}

func (m model) viewDetail() string {
	lines := objectDetailLines(m.detail)
	m.detailScroll = clamp(m.detailScroll, 0, m.maxDetailScroll())
	visible := visibleHeight(m.height)
	end := min(len(lines), m.detailScroll+visible)
	return strings.Join(lines[m.detailScroll:end], "\n") + "\n"
}

func objectDetailLines(detail objectDetail) []string {
	lines := []string{
		"Key: " + objectPathStyle.Render(detail.Object.Key),
		"Size: " + formatBytes(detail.Object.Size),
	}
	if detail.Object.ContentType != "" {
		lines = append(lines, "Content-Type: "+detail.Object.ContentType)
	}
	if detail.Object.ETag != "" {
		lines = append(lines, "ETag: "+detail.Object.ETag)
	}
	if !detail.Object.LastModified.IsZero() {
		lines = append(lines, "Last-Modified: "+detail.Object.LastModified.Format(time.RFC3339))
	}
	if len(detail.Metadata) > 0 {
		lines = append(lines, "", metadataTitleStyle.Render("Metadata:"))
		keys := make([]string, 0, len(detail.Metadata))
		for key := range detail.Metadata {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("  %s: %s", metadataKeyStyle.Render(key), detail.Metadata[key]))
		}
	}
	lines = append(lines, "", previewTitleStyle.Render("Preview:"))
	if detail.Binary {
		lines = append(lines, binaryNoticeStyle.Render("binary object; showing hex preview"))
	}
	if detail.Preview == "" {
		lines = append(lines, "(empty)")
	} else {
		lines = append(lines, strings.Split(detail.Preview, "\n")...)
	}
	if detail.Truncated {
		lines = append(lines, "", fmt.Sprintf("[preview truncated at %s]", formatBytes(detail.PreviewLen)))
	}
	return lines
}

func (m model) loadBuckets() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		buckets, err := m.service.ListBuckets(ctx)
		return bucketsLoadedMsg{buckets: buckets, err: err}
	}
}

func (m model) loadObjects(bucket string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		objects, err := m.service.ListObjects(ctx, bucket)
		return objectsLoadedMsg{bucket: bucket, objects: objects, err: err}
	}
}

func (m model) loadDetail(bucket, key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		detail, err := m.service.InspectObject(ctx, bucket, key, previewBytes)
		return detailLoadedMsg{detail: detail, err: err}
	}
}

func currentPath(node *treeNode) string {
	if node == nil || node.Path == "" {
		return "/"
	}
	return "/" + strings.TrimPrefix(node.Path, "/")
}

func clampCursor(cursor, count int) int {
	if count == 0 {
		return 0
	}
	return clamp(cursor, 0, count-1)
}

func visibleHeight(height int) int {
	if height <= 8 {
		return 20
	}
	return max(1, height-8)
}

func (m model) maxDetailScroll() int {
	return max(0, len(objectDetailLines(m.detail))-visibleHeight(m.height))
}

func wrapCursor(cursor, delta, count int) int {
	if count == 0 {
		return 0
	}
	next := (cursor + delta) % count
	if next < 0 {
		next += count
	}
	return next
}

func clampListScroll(scroll, cursor, count, visible int) int {
	if count == 0 {
		return 0
	}
	if visible <= 0 {
		visible = 1
	}
	maxScroll := max(0, count-visible)
	scroll = clamp(scroll, 0, maxScroll)
	if cursor < scroll {
		return cursor
	}
	if cursor >= scroll+visible {
		return clamp(cursor-visible+1, 0, maxScroll)
	}
	return scroll
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
