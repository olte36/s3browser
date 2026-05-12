package main

import "testing"

func TestBuildObjectTree(t *testing.T) {
	objects := []objectItem{
		{Key: "root.txt", Size: 10},
		{Key: "logs/2026/01.txt", Size: 20},
		{Key: "logs/2026/02.txt", Size: 30},
		{Key: "images/logo.png", Size: 40},
	}

	root := buildObjectTree(objects)
	entries := listChildren(root)
	want := []string{"images/", "logs/", "root.txt"}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}
	for i := range want {
		if entries[i].Label != want[i] {
			t.Fatalf("entry %d = %q, want %q", i, entries[i].Label, want[i])
		}
	}

	logs := root.Children["logs"]
	year := logs.Children["2026"]
	if got := listChildren(year); len(got) != 2 {
		t.Fatalf("got %d children under logs/2026, want 2", len(got))
	}
	if year.Children["01.txt"].Object == nil || year.Children["01.txt"].Object.Key != "logs/2026/01.txt" {
		t.Fatal("object metadata not attached to leaf")
	}
}

func TestBuildObjectTreeEmpty(t *testing.T) {
	root := buildObjectTree(nil)
	if got := listChildren(root); len(got) != 0 {
		t.Fatalf("got %d entries, want 0", len(got))
	}
}

func TestBuildObjectTreeDirectoryPlaceholder(t *testing.T) {
	root := buildObjectTree([]objectItem{{Key: "folder/"}, {Key: "folder/file.txt", Size: 1}})
	folder := root.Children["folder"]
	if folder == nil {
		t.Fatal("folder node not created")
	}
	if folder.Kind != nodeFolder {
		t.Fatalf("folder kind = %v, want folder", folder.Kind)
	}
	if got := listChildren(folder); len(got) != 1 || got[0].Label != "file.txt" {
		t.Fatalf("unexpected folder children: %#v", got)
	}
}

func TestBuildObjectTreeObjectPrefixCollision(t *testing.T) {
	root := buildObjectTree([]objectItem{{Key: "dir", Size: 1}, {Key: "dir/file.txt", Size: 2}})
	dir := root.Children["dir"]
	if dir == nil {
		t.Fatal("dir node not created")
	}
	if dir.Kind != nodeFolder {
		t.Fatalf("dir kind = %v, want folder", dir.Kind)
	}
	if got := listChildren(dir); len(got) != 1 || got[0].Label != "file.txt" {
		t.Fatalf("unexpected dir children: %#v", got)
	}
}
