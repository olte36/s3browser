package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
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
	ctx     context.Context
	storage string
	service s3Service

	mode     viewMode
	loading  bool
	err      error
	status   string
	statusID int

	width  int
	height int

	buckets      []bucketItem
	bucketCursor int
	bucketScroll int
	activeBucket string

	objectCache           map[string]objectItem
	loadedPrefix          map[string]bool
	root                  *treeNode
	current               *treeNode
	pathStack             []*treeNode
	objectCursor          int
	objectScroll          int
	objectLoadID          int
	objectLoadCancel      context.CancelFunc
	objectLoadProgress    *atomic.Int64
	objectLoadCount       int
	objectLoadInterrupted bool

	detail       objectDetail
	detailScroll int
	detailWrap   bool
}

type bucketsLoadedMsg struct {
	buckets []bucketItem
	err     error
}

type objectsLoadedMsg struct {
	loadID  int
	bucket  string
	prefix  string
	objects []objectItem
	err     error
}

type objectLoadProgressMsg struct {
	loadID int
}

type detailLoadedMsg struct {
	detail objectDetail
	err    error
}

type copiedMsg struct {
	label string
	err   error
}

type clearStatusMsg struct {
	id int
}

func newModel(ctx context.Context, storage string, service s3Service) model {
	return model{
		ctx:          ctx,
		storage:      storage,
		service:      service,
		mode:         viewBuckets,
		loading:      true,
		objectCache:  map[string]objectItem{},
		loadedPrefix: map[string]bool{},
	}
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
	case copiedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = ""
			return m, nil
		}
		m.err = nil
		m.statusID++
		m.status = "Copied " + msg.label
		return m, clearStatusAfter(m.statusID)
	case clearStatusMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
	case objectLoadProgressMsg:
		if msg.loadID == m.objectLoadID && m.loading && m.objectLoadProgress != nil {
			m.objectLoadCount = int(m.objectLoadProgress.Load())
			return m, objectLoadProgressTick(msg.loadID)
		}
	case bucketsLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.status = ""
		m.buckets = msg.buckets
		m.bucketCursor = clampCursor(m.bucketCursor, len(m.buckets))
		m.bucketScroll = clampListScroll(m.bucketScroll, m.bucketCursor, len(m.buckets), visibleHeight(m.height))
	case objectsLoadedMsg:
		if msg.loadID != 0 && msg.loadID != m.objectLoadID {
			return m, nil
		}
		m.loading = false
		m.objectLoadCancel = nil
		m.objectLoadProgress = nil
		m.objectLoadCount = len(msg.objects)
		m.objectLoadInterrupted = false
		m.err = nil
		m.status = ""
		if msg.err == nil || errors.Is(msg.err, context.Canceled) {
			oldBucket := m.activeBucket
			m.mode = viewObjects
			m.activeBucket = msg.bucket
			if m.objectCache == nil || oldBucket != msg.bucket {
				m.objectCache = map[string]objectItem{}
			}
			if m.loadedPrefix == nil || oldBucket != msg.bucket {
				m.loadedPrefix = map[string]bool{}
			}
			m.mergeObjects(msg.prefix, msg.objects)
			if msg.err == nil {
				m.loadedPrefix[normalizePrefix(msg.prefix)] = true
			} else {
				m.objectLoadInterrupted = true
			}
			m.rebuildObjectTree()
			m.current = m.findNode(normalizePrefix(msg.prefix))
			if m.current == nil {
				m.current = m.root
			}
			m.pathStack = pathStackForPrefix(m.root, normalizePrefix(msg.prefix))
			m.objectCursor = 0
			m.objectScroll = 0
		} else {
			m.err = msg.err
		}
	case detailLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.status = ""
		if msg.err == nil {
			m.mode = viewDetail
			m.detail = msg.detail
			m.detailScroll = 0
			m.detailWrap = false
		}
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "x":
		if m.loading && m.objectLoadCancel != nil {
			m.objectLoadCancel()
			m.status = "Canceling object load..."
			return m, nil
		}
	case "r":
		m.cancelObjectLoad()
		m.loading = true
		m.err = nil
		m.status = ""
		switch m.mode {
		case viewBuckets:
			return m, m.loadBuckets()
		case viewObjects:
			return m, m.startObjectLoad(m.activeBucket, currentObjectPath(m.current))
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
	case "c":
		switch m.mode {
		case viewObjects:
			return m, copyText("URI", s3URI(m.activeBucket, currentObjectPath(m.current)))
		case viewDetail:
			return m, copyText("URI", s3URI(m.activeBucket, m.detail.Object.Key))
		}
	case "m":
		if m.mode == viewDetail {
			return m, copyText("metadata", metadataText(m.detail.Metadata))
		}
	case "p":
		if m.mode == viewDetail {
			return m, copyText("preview", m.detail.Preview)
		}
	case "w":
		if m.mode == viewDetail && !m.detail.Binary {
			m.detailWrap = !m.detailWrap
			m.detailScroll = clamp(m.detailScroll, 0, m.maxDetailScroll())
		}
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
	m.status = ""
	switch m.mode {
	case viewBuckets:
		if len(m.buckets) == 0 {
			return m, nil
		}
		bucket := m.buckets[m.bucketCursor].Name
		m.loading = true
		m.err = nil
		return m, m.startObjectLoad(bucket, "")
	case viewObjects:
		entries := listChildren(m.current)
		if len(entries) == 0 {
			return m, nil
		}
		node := entries[m.objectCursor].Node
		if node.Kind == nodeFolder {
			if !m.loadedPrefix[normalizePrefix(node.Path)] {
				m.loading = true
				m.err = nil
				return m, m.startObjectLoad(m.activeBucket, node.Path)
			}
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
	m.status = ""
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
		b.WriteString(m.loadingLine() + "\n")
	}
	if m.err != nil {
		b.WriteString("Error: " + m.err.Error() + "\n\n")
	}
	if m.status != "" {
		b.WriteString(m.status + "\n\n")
	}
	if !m.loading && m.mode == viewObjects && m.objectLoadCount > 0 {
		b.WriteString(m.objectLoadSummaryLine() + "\n\n")
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
	b.WriteString(m.footer())
	return b.String()
}

func (m model) header() string {
	prefix := "s3browser - " + m.storage
	switch m.mode {
	case viewBuckets:
		return prefix + " - buckets"
	case viewObjects:
		return prefix + "\nURI: " + headerPathStyle.Render(s3URI(m.activeBucket, currentObjectPath(m.current)))
	case viewDetail:
		return prefix + "\nURI: " + headerPathStyle.Render(s3URI(m.activeBucket, m.detail.Object.Key))
	default:
		return prefix
	}
}

func (m model) footer() string {
	if m.loading && m.objectLoadCancel != nil {
		return "x cancel load  q quit"
	}
	switch m.mode {
	case viewObjects:
		return "enter open/view  c copy uri  backspace back  r reload  q quit"
	case viewDetail:
		wrapState := "off"
		if m.detailWrap {
			wrapState = "on"
		}
		return "w wrap " + wrapState + "  c copy uri  m copy metadata  p copy preview  backspace back  r reload  q quit"
	default:
		return "enter open/view  backspace back  r reload  q quit"
	}
}

func (m model) loadingLine() string {
	if m.objectLoadCancel != nil {
		return fmt.Sprintf("Loading objects... %d loaded (press x to cancel)", m.objectLoadCount)
	}
	return "Loading..."
}

func (m model) objectLoadSummaryLine() string {
	if m.objectLoadInterrupted {
		return fmt.Sprintf("Objects loaded: %d (interrupted)", m.objectLoadCount)
	}
	return fmt.Sprintf("Objects loaded: %d", m.objectLoadCount)
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
		b.WriteString(fmt.Sprintf("%s %s %s %s\n", cursor, styledObjectTimestamp(entry.Node), styledObjectSize(entry.Node), objectEntryLabel(entry)))
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
	lines := m.objectDetailLines()
	m.detailScroll = clamp(m.detailScroll, 0, m.maxDetailScroll())
	visible := visibleHeight(m.height)
	end := min(len(lines), m.detailScroll+visible)
	return strings.Join(lines[m.detailScroll:end], "\n") + "\n"
}

func objectDetailLines(bucket string, detail objectDetail) []string {
	return objectDetailLinesWithOptions(bucket, detail, 0, false)
}

func (m model) objectDetailLines() []string {
	return objectDetailLinesWithOptions(m.activeBucket, m.detail, m.width, m.detailWrap)
}

func objectDetailLinesWithOptions(bucket string, detail objectDetail, width int, wrap bool) []string {
	lines := []string{
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
	} else if detail.Binary {
		lines = append(lines, strings.Split(detail.Preview, "\n")...)
	} else {
		lines = append(lines, numberedPreviewLines(detail.Preview, width, wrap)...)
	}
	if detail.Truncated {
		lines = append(lines, "", fmt.Sprintf("[preview truncated at %s]", formatBytes(detail.PreviewLen)))
	}
	return lines
}

func numberedPreviewLines(preview string, width int, wrap bool) []string {
	rawLines := strings.Split(preview, "\n")
	if len(rawLines) > 1 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	numberWidth := len(fmt.Sprintf("%d", max(1, len(rawLines))))
	prefixWidth := numberWidth + len(" | ")
	contentWidth := 0
	if wrap && width > prefixWidth {
		contentWidth = width - prefixWidth
	}

	lines := make([]string, 0, len(rawLines))
	for i, line := range rawLines {
		prefix := fmt.Sprintf("%*d | ", numberWidth, i+1)
		if contentWidth <= 0 {
			lines = append(lines, prefix+line)
			continue
		}
		wrapped := wrapTextLine(line, contentWidth)
		for j, part := range wrapped {
			if j == 0 {
				lines = append(lines, prefix+part)
			} else {
				lines = append(lines, strings.Repeat(" ", numberWidth)+" | "+part)
			}
		}
	}
	return lines
}

func wrapTextLine(line string, width int) []string {
	runes := []rune(line)
	if width <= 0 || len(runes) <= width {
		return []string{line}
	}
	parts := make([]string, 0, (len(runes)/width)+1)
	for len(runes) > width {
		parts = append(parts, string(runes[:width]))
		runes = runes[width:]
	}
	parts = append(parts, string(runes))
	return parts
}

func s3URI(bucket, key string) string {
	if key == "" {
		return "s3://" + bucket
	}
	return "s3://" + bucket + "/" + strings.TrimPrefix(key, "/")
}

func objectEntryLabel(entry navEntry) string {
	return entry.Label
}

func currentObjectPath(node *treeNode) string {
	if node == nil {
		return ""
	}
	return node.Path
}

func metadataText(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+": "+metadata[key])
	}
	return strings.Join(lines, "\n")
}

func copyText(label, text string) tea.Cmd {
	return func() tea.Msg {
		_, err := fmt.Fprint(os.Stdout, osc52.New(text).String())
		return copiedMsg{label: label, err: err}
	}
}

func clearStatusAfter(id int) tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{id: id}
	})
}

func objectLoadProgressTick(loadID int) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return objectLoadProgressMsg{loadID: loadID}
	})
}

func (m model) loadBuckets() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		buckets, err := m.service.ListBuckets(ctx)
		return bucketsLoadedMsg{buckets: buckets, err: err}
	}
}

func (m *model) startObjectLoad(bucket, prefix string) tea.Cmd {
	m.cancelObjectLoad()
	m.objectLoadID++
	m.objectLoadCount = 0
	m.objectLoadInterrupted = false
	progress := &atomic.Int64{}
	m.objectLoadProgress = progress
	ctx, cancel := context.WithCancel(m.ctx)
	m.objectLoadCancel = cancel
	loadID := m.objectLoadID
	return tea.Batch(
		m.loadObjects(ctx, loadID, bucket, prefix, progress),
		objectLoadProgressTick(loadID),
	)
}

func (m *model) cancelObjectLoad() {
	if m.objectLoadCancel != nil {
		m.objectLoadCancel()
		m.objectLoadCancel = nil
	}
}

func (m model) loadObjects(ctx context.Context, loadID int, bucket, prefix string, progress *atomic.Int64) tea.Cmd {
	return func() tea.Msg {
		objects, err := m.service.ListObjects(ctx, bucket, prefix, func(count int) {
			if progress != nil {
				progress.Store(int64(count))
			}
		})
		return objectsLoadedMsg{loadID: loadID, bucket: bucket, prefix: prefix, objects: objects, err: err}
	}
}

func (m model) loadDetail(bucket, key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		detail, err := m.service.InspectObject(ctx, bucket, key, previewBytes)
		return detailLoadedMsg{detail: detail, err: err}
	}
}

func (m *model) mergeObjects(prefix string, objects []objectItem) {
	if m.objectCache == nil {
		m.objectCache = map[string]objectItem{}
	}
	prefix = normalizePrefix(prefix)
	for key, object := range m.objectCache {
		if isDirectChildKey(prefix, key, object.IsPrefix) {
			delete(m.objectCache, key)
		}
	}
	for _, object := range objects {
		if object.Key == "" {
			continue
		}
		m.objectCache[object.Key] = object
	}
}

func (m *model) rebuildObjectTree() {
	objects := make([]objectItem, 0, len(m.objectCache))
	for _, object := range m.objectCache {
		objects = append(objects, object)
	}
	m.root = buildObjectTree(objects)
}

func (m model) findNode(prefix string) *treeNode {
	prefix = normalizePrefix(prefix)
	if prefix == "" {
		return m.root
	}
	if m.root == nil {
		return nil
	}
	current := m.root
	for _, part := range strings.Split(strings.Trim(prefix, "/"), "/") {
		if part == "" || current.Children == nil {
			continue
		}
		current = current.Children[part]
		if current == nil {
			return nil
		}
	}
	return current
}

func pathStackForPrefix(root *treeNode, prefix string) []*treeNode {
	prefix = normalizePrefix(prefix)
	if root == nil || prefix == "" {
		return nil
	}
	current := root
	stack := []*treeNode{root}
	parts := strings.Split(strings.Trim(prefix, "/"), "/")
	for i, part := range parts {
		if part == "" || current.Children == nil {
			return stack[:0]
		}
		next := current.Children[part]
		if next == nil {
			return stack[:0]
		}
		if i < len(parts)-1 {
			stack = append(stack, next)
		}
		current = next
	}
	return stack
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func isDirectChildKey(prefix, key string, isPrefix bool) bool {
	key = strings.TrimPrefix(key, "/")
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	rest := strings.TrimPrefix(key, prefix)
	if rest == "" {
		return false
	}
	if isPrefix {
		rest = strings.TrimSuffix(rest, "/")
	}
	return !strings.Contains(rest, "/")
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
	return max(0, len(m.objectDetailLines())-visibleHeight(m.height))
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
