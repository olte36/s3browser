package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainTerminalText(value string) string {
	return ansiEscapePattern.ReplaceAllString(value, "")
}

type fakeService struct {
	buckets []bucketItem
	objects []objectItem
	detail  objectDetail
	err     error
}

func (f fakeService) ListBuckets(context.Context) ([]bucketItem, error) {
	return f.buckets, f.err
}

func (f fakeService) ListObjects(context.Context, string, string) ([]objectItem, error) {
	return f.objects, f.err
}

func (f fakeService) InspectObject(context.Context, string, string, int64) (objectDetail, error) {
	return f.detail, f.err
}

func TestModelNavigation(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(bucketsLoadedMsg{buckets: []bucketItem{{Name: "alpha"}}})
	m = modelAfter.(model)

	modelAfter, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelAfter.(model)
	if cmd == nil || !m.loading {
		t.Fatal("expected bucket enter to start object load")
	}

	modelAfter, _ = m.Update(objectsLoadedMsg{
		bucket: "alpha",
		objects: []objectItem{
			{Key: "dir/", IsPrefix: true},
		},
	})
	m = modelAfter.(model)
	if m.mode != viewObjects || m.activeBucket != "alpha" {
		t.Fatalf("unexpected object state: mode=%v bucket=%q", m.mode, m.activeBucket)
	}

	modelAfter, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelAfter.(model)
	if cmd == nil || !m.loading {
		t.Fatal("expected prefix enter to start scoped object load")
	}
	lazyMsg := cmd().(objectsLoadedMsg)
	if lazyMsg.bucket != "alpha" || lazyMsg.prefix != "dir/" {
		t.Fatalf("lazy load requested bucket=%q prefix=%q, want alpha dir/", lazyMsg.bucket, lazyMsg.prefix)
	}

	modelAfter, _ = m.Update(objectsLoadedMsg{
		bucket: "alpha",
		prefix: "dir/",
		objects: []objectItem{
			{Key: "dir/file.txt", Size: 3},
		},
	})
	m = modelAfter.(model)
	if currentPath(m.current) != "/dir/" {
		t.Fatalf("current path = %q, want /dir/", currentPath(m.current))
	}

	modelAfter, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelAfter.(model)
	if cmd == nil || !m.loading {
		t.Fatal("expected object enter to start detail load")
	}

	modelAfter, _ = m.Update(detailLoadedMsg{detail: objectDetail{Object: objectItem{Key: "dir/file.txt"}, Preview: "hello"}})
	m = modelAfter.(model)
	if m.mode != viewDetail {
		t.Fatalf("mode = %v, want detail", m.mode)
	}

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = modelAfter.(model)
	if m.mode != viewObjects {
		t.Fatalf("mode = %v, want objects", m.mode)
	}
}

func TestModelRendersErrors(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(bucketsLoadedMsg{err: errors.New("no credentials")})
	m = modelAfter.(model)
	if !strings.Contains(m.View(), "no credentials") {
		t.Fatalf("view does not contain error: %s", m.View())
	}
}

func TestModelCursorWrapsAroundBucketsAndObjects(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(bucketsLoadedMsg{buckets: []bucketItem{{Name: "a"}, {Name: "b"}, {Name: "c"}}})
	m = modelAfter.(model)

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = modelAfter.(model)
	if m.bucketCursor != 2 {
		t.Fatalf("bucket cursor = %d, want 2", m.bucketCursor)
	}

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = modelAfter.(model)
	if m.bucketCursor != 0 {
		t.Fatalf("bucket cursor = %d, want 0", m.bucketCursor)
	}

	modelAfter, _ = m.Update(objectsLoadedMsg{
		bucket: "a",
		objects: []objectItem{
			{Key: "one.txt"},
			{Key: "two.txt"},
		},
	})
	m = modelAfter.(model)

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = modelAfter.(model)
	if m.objectCursor != 1 {
		t.Fatalf("object cursor = %d, want 1", m.objectCursor)
	}

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = modelAfter.(model)
	if m.objectCursor != 0 {
		t.Fatalf("object cursor = %d, want 0", m.objectCursor)
	}
}

func TestBucketListScrollsToSelectedBucket(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	m.height = 12
	buckets := make([]bucketItem, 15)
	for i := range buckets {
		buckets[i] = bucketItem{Name: fmt.Sprintf("bucket-%02d", i)}
	}
	modelAfter, _ := m.Update(bucketsLoadedMsg{buckets: buckets})
	m = modelAfter.(model)

	for range 12 {
		modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = modelAfter.(model)
	}

	view := plainTerminalText(m.viewBuckets())
	if !strings.Contains(view, "bucket-12") {
		t.Fatalf("selected bucket is not visible:\n%s", view)
	}
	if strings.Contains(view, "bucket-00") {
		t.Fatalf("bucket list did not scroll:\n%s", view)
	}
}

func TestObjectListScrollsToSelectedObject(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	m.height = 12
	objects := make([]objectItem, 15)
	for i := range objects {
		objects[i] = objectItem{Key: fmt.Sprintf("object-%02d.txt", i)}
	}
	modelAfter, _ := m.Update(objectsLoadedMsg{bucket: "a", objects: objects})
	m = modelAfter.(model)

	for range 12 {
		modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = modelAfter.(model)
	}

	view := plainTerminalText(m.viewObjects())
	if !strings.Contains(view, "object-12.txt") {
		t.Fatalf("selected object is not visible:\n%s", view)
	}
	if strings.Contains(view, "object-00.txt") {
		t.Fatalf("object list did not scroll:\n%s", view)
	}
}

func TestBucketRowsRenderTimestampBeforeName(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(bucketsLoadedMsg{buckets: []bucketItem{{
		Name:         "archive",
		CreationDate: time.Date(2026, 5, 12, 9, 10, 11, 0, time.UTC),
	}}})
	m = modelAfter.(model)

	line := strings.TrimSpace(plainTerminalText(m.viewBuckets()))
	timestampIndex := strings.Index(line, "2026-05-12 09:10:11")
	nameIndex := strings.Index(line, "archive")
	if timestampIndex < 0 || nameIndex < 0 || timestampIndex > nameIndex {
		t.Fatalf("bucket row should render timestamp before name: %q", line)
	}
}

func TestObjectRowsRenderTimestampAndSizeBeforeName(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(objectsLoadedMsg{
		bucket: "archive",
		prefix: "reports/",
		objects: []objectItem{{
			Key:          "reports/may.csv",
			Size:         1536,
			LastModified: time.Date(2026, 5, 12, 9, 10, 11, 0, time.UTC),
		}},
	})
	m = modelAfter.(model)
	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelAfter.(model)

	line := strings.TrimSpace(plainTerminalText(m.viewObjects()))
	timestampIndex := strings.Index(line, "2026-05-12 09:10:11")
	sizeIndex := strings.Index(line, "1.5 KiB")
	nameIndex := strings.Index(line, "may.csv")
	if timestampIndex < 0 || sizeIndex < 0 || nameIndex < 0 || !(timestampIndex < sizeIndex && sizeIndex < nameIndex) {
		t.Fatalf("object row should render timestamp and size before name: %q", line)
	}
}

func TestPrefixRowsRenderPrefixInTimestampColumn(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(objectsLoadedMsg{
		bucket: "archive",
		prefix: "",
		objects: []objectItem{{
			Key:      "reports/",
			IsPrefix: true,
		}},
	})
	m = modelAfter.(model)

	line := strings.TrimSpace(plainTerminalText(m.viewObjects()))
	prefixIndex := strings.Index(line, "PREFIX")
	nameIndex := strings.Index(line, "reports/")
	if prefixIndex < 0 || nameIndex < 0 || prefixIndex > nameIndex {
		t.Fatalf("prefix row should render PREFIX before name: %q", line)
	}
	if strings.Contains(line, "2026-05-12") || strings.Contains(line, "1.5 KiB") {
		t.Fatalf("prefix row should not render object timestamp or size: %q", line)
	}
}

func TestObjectsHeaderShowsStorageAndURIOnSeparateLines(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(objectsLoadedMsg{
		bucket:  "archive",
		objects: []objectItem{{Key: "reports/may.csv"}},
	})
	m = modelAfter.(model)

	header := plainTerminalText(m.header())
	if !strings.Contains(header, "AWS\nURI: s3://archive") {
		t.Fatalf("header should render storage and URI on separate lines: %q", header)
	}

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelAfter.(model)
	modelAfter, _ = m.Update(objectsLoadedMsg{
		bucket:  "archive",
		prefix:  "reports/",
		objects: []objectItem{{Key: "reports/may.csv"}},
	})
	m = modelAfter.(model)
	header = plainTerminalText(m.header())
	if !strings.Contains(header, "AWS\nURI: s3://archive/reports/") {
		t.Fatalf("header should render prefix URI on separate line: %q", header)
	}
}

func TestModelDetailScrollClampsAtEnd(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	m.mode = viewDetail
	m.height = 12
	m.detail = objectDetail{
		Object:  objectItem{Key: "long.txt", Size: 1},
		Preview: strings.Repeat("line\n", 30),
	}

	for range 50 {
		modelAfter, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = modelAfter.(model)
	}

	if m.detailScroll != m.maxDetailScroll() {
		t.Fatalf("detail scroll = %d, want %d", m.detailScroll, m.maxDetailScroll())
	}
}

func TestDetailHeaderHighlightsObjectPath(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	m.mode = viewDetail
	m.activeBucket = "bucket"
	m.detail = objectDetail{Object: objectItem{Key: "path/object.txt"}}

	header := m.header()
	if !strings.Contains(header, "AWS") || !strings.Contains(header, "s3://bucket/path/object.txt") {
		t.Fatalf("header missing object path: %q", header)
	}
	if !strings.Contains(header, "\n") {
		t.Fatalf("header should render URI on a new line: %q", header)
	}
}

func TestObjectDetailLinesRenderMetadataForBinaryPreview(t *testing.T) {
	lines := strings.Join(objectDetailLines("bucket", objectDetail{
		Object:   objectItem{Key: "archive/data.bin", Size: 4},
		Metadata: map[string]string{"x-amz-meta-owner": "ops"},
		Preview:  "00000000  00 01 ff 10                                      |....|\n",
		Binary:   true,
	}), "\n")

	for _, want := range []string{"Metadata:", "x-amz-meta-owner", "ops", "Preview:", "binary object; showing hex preview", "00000000"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("detail lines missing %q:\n%s", want, lines)
		}
	}
	for _, unwanted := range []string{"URI:", "s3://bucket/archive/data.bin", "Key:", "archive/data.bin"} {
		if strings.Contains(lines, unwanted) {
			t.Fatalf("detail lines should not duplicate object URI/key %q:\n%s", unwanted, lines)
		}
	}
}

func TestObjectDetailLinesNumberTextPreview(t *testing.T) {
	lines := strings.Join(objectDetailLines("bucket", objectDetail{
		Object:  objectItem{Key: "notes.txt", Size: 11},
		Preview: "first\nsecond\n",
	}), "\n")

	for _, want := range []string{"1 | first", "2 | second"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("detail lines missing numbered preview %q:\n%s", want, lines)
		}
	}
}

func TestDetailWrapToggleWrapsTextPreview(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	m.mode = viewDetail
	m.width = 12
	m.height = 20
	m.detail = objectDetail{
		Object:  objectItem{Key: "long.txt", Size: 10},
		Preview: "abcdefghij",
	}

	view := plainTerminalText(m.viewDetail())
	if !strings.Contains(view, "1 | abcdefghij") {
		t.Fatalf("unwrapped detail view missing original line:\n%s", view)
	}

	modelAfter, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = modelAfter.(model)
	view = plainTerminalText(m.viewDetail())
	if !strings.Contains(view, "1 | abcdefgh") || !strings.Contains(view, "  | ij") {
		t.Fatalf("wrapped detail view missing wrapped numbered lines:\n%s", view)
	}
	if !strings.Contains(m.footer(), "w wrap on") {
		t.Fatalf("footer missing wrap state: %q", m.footer())
	}
}

func TestObjectListCopyKeyReturnsCopyCommand(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	modelAfter, _ := m.Update(objectsLoadedMsg{
		bucket:  "archive",
		objects: []objectItem{{Key: "reports/may.csv"}},
	})
	m = modelAfter.(model)

	modelAfter, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = modelAfter.(model)
	if cmd == nil {
		t.Fatal("expected copy command")
	}

	modelAfter, _ = m.Update(copiedMsg{label: "URI"})
	m = modelAfter.(model)
	if !strings.Contains(m.View(), "Copied URI") {
		t.Fatalf("view missing copy status:\n%s", m.View())
	}

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelAfter.(model)
	modelAfter, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("expected prefix copy command")
	}
}

func TestDetailCopyKeysReturnCopyCommands(t *testing.T) {
	m := newModel(context.Background(), "AWS", fakeService{})
	m.mode = viewDetail
	m.activeBucket = "bucket"
	m.detail = objectDetail{
		Object:   objectItem{Key: "path/object.txt"},
		Metadata: map[string]string{"owner": "ops"},
		Preview:  "hello",
	}

	for _, key := range []rune{'c', 'm', 'p'} {
		modelAfter, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = modelAfter.(model)
		if cmd == nil {
			t.Fatalf("expected copy command for %q", key)
		}
	}

	modelAfter, cmd := m.Update(copiedMsg{label: "URI"})
	m = modelAfter.(model)
	if cmd == nil {
		t.Fatal("expected status clear command")
	}
	if !strings.Contains(m.View(), "Copied URI") {
		t.Fatalf("view missing copy status:\n%s", m.View())
	}

	modelAfter, _ = m.Update(clearStatusMsg{id: m.statusID})
	m = modelAfter.(model)
	if strings.Contains(m.View(), "Copied URI") {
		t.Fatalf("copy status should be cleared:\n%s", m.View())
	}
}
