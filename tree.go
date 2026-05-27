package main

import (
	"sort"
	"strings"
)

const (
	// nodeFolder marks a tree node that represents a prefix.
	nodeFolder = iota

	// nodeObject marks a tree node that represents a concrete object.
	nodeObject
)

// nodeKind identifies whether a tree node is a prefix or an object.
type nodeKind int

// treeNode represents one navigable item in the object hierarchy.
type treeNode struct {
	Name     string
	Path     string
	Kind     nodeKind
	Object   *objectItem
	Children map[string]*treeNode
}

// navEntry pairs a rendered label with the tree node it selects.
type navEntry struct {
	Label string
	Node  *treeNode
}

// buildObjectTree converts flat S3 object keys into a navigable hierarchy.
func buildObjectTree(objects []objectItem) *treeNode {
	root := &treeNode{Name: "/", Children: map[string]*treeNode{}}
	for i := range objects {
		key := strings.TrimPrefix(objects[i].Key, "/")
		if key == "" {
			continue
		}
		parts := strings.Split(key, "/")
		current := root
		for idx, part := range parts {
			if part == "" {
				continue
			}
			if current.Children == nil {
				current.Children = map[string]*treeNode{}
			}
			isLeaf := idx == len(parts)-1
			if isLeaf && (objects[i].IsPrefix || strings.HasSuffix(objects[i].Key, "/")) {
				isLeaf = false
			}
			child, ok := current.Children[part]
			if !ok {
				child = &treeNode{
					Name:     part,
					Path:     joinKey(current.Path, part, !isLeaf),
					Kind:     nodeFolder,
					Children: map[string]*treeNode{},
				}
				current.Children[part] = child
			}
			if !isLeaf && child.Kind == nodeObject {
				child.Kind = nodeFolder
				child.Object = nil
				child.Children = map[string]*treeNode{}
				child.Path = joinKey(current.Path, part, true)
			}
			if isLeaf {
				obj := objects[i]
				child.Kind = nodeObject
				child.Path = obj.Key
				child.Object = &obj
				child.Children = nil
			}
			current = child
		}
	}
	return root
}

// listChildren returns sorted navigation entries for a tree node.
func listChildren(node *treeNode) []navEntry {
	if node == nil || len(node.Children) == 0 {
		return nil
	}
	names := make([]string, 0, len(node.Children))
	for name := range node.Children {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left := node.Children[names[i]]
		right := node.Children[names[j]]
		if left.Kind != right.Kind {
			return left.Kind == nodeFolder
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
	entries := make([]navEntry, 0, len(names))
	for _, name := range names {
		child := node.Children[name]
		label := child.Name
		if child.Kind == nodeFolder {
			label += "/"
		}
		entries = append(entries, navEntry{Label: label, Node: child})
	}
	return entries
}

// joinKey joins a prefix and name into an object key or folder prefix.
func joinKey(prefix, name string, folder bool) string {
	prefix = strings.Trim(prefix, "/")
	path := name
	if prefix != "" {
		path = prefix + "/" + name
	}
	if folder {
		path += "/"
	}
	return path
}
