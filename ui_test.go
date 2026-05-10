package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeService struct {
	buckets []bucketItem
	objects []objectItem
	detail  objectDetail
	err     error
}

func (f fakeService) ListBuckets(context.Context) ([]bucketItem, error) {
	return f.buckets, f.err
}

func (f fakeService) ListObjects(context.Context, string) ([]objectItem, error) {
	return f.objects, f.err
}

func (f fakeService) InspectObject(context.Context, string, string, int64) (objectDetail, error) {
	return f.detail, f.err
}

func TestModelNavigation(t *testing.T) {
	m := newModel(fakeService{})
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
			{Key: "dir/file.txt", Size: 3},
		},
	})
	m = modelAfter.(model)
	if m.mode != viewObjects || m.activeBucket != "alpha" {
		t.Fatalf("unexpected object state: mode=%v bucket=%q", m.mode, m.activeBucket)
	}

	modelAfter, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
	m := newModel(fakeService{})
	modelAfter, _ := m.Update(bucketsLoadedMsg{err: errors.New("no credentials")})
	m = modelAfter.(model)
	if !strings.Contains(m.View(), "no credentials") {
		t.Fatalf("view does not contain error: %s", m.View())
	}
}

func TestModelCursorWrapsAroundBucketsAndObjects(t *testing.T) {
	m := newModel(fakeService{})
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

func TestModelDetailScrollClampsAtEnd(t *testing.T) {
	m := newModel(fakeService{})
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
	m := newModel(fakeService{})
	m.mode = viewDetail
	m.activeBucket = "bucket"
	m.detail = objectDetail{Object: objectItem{Key: "path/object.txt"}}

	header := m.header()
	if !strings.Contains(header, "bucket/path/object.txt") {
		t.Fatalf("header missing object path: %q", header)
	}
}

func TestObjectDetailLinesRenderMetadataForBinaryPreview(t *testing.T) {
	lines := strings.Join(objectDetailLines(objectDetail{
		Object:   objectItem{Key: "archive/data.bin", Size: 4},
		Metadata: map[string]string{"x-amz-meta-owner": "ops"},
		Preview:  "00000000  00 01 ff 10                                      |....|\n",
		Binary:   true,
	}), "\n")

	for _, want := range []string{"archive/data.bin", "Metadata:", "x-amz-meta-owner", "ops", "Preview:", "binary object; showing hex preview", "00000000"} {
		if !strings.Contains(lines, want) {
			t.Fatalf("detail lines missing %q:\n%s", want, lines)
		}
	}
}
