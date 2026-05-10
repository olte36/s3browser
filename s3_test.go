package main

import (
	"strings"
	"testing"
)

func TestRenderPreviewSanitizesTextControlBytes(t *testing.T) {
	preview, binary := renderPreview([]byte("hello\x1b[2Jworld"))
	if binary {
		t.Fatal("text preview marked as binary")
	}
	if strings.Contains(preview, "\x1b") {
		t.Fatalf("preview contains raw escape byte: %q", preview)
	}
	if !strings.Contains(preview, "hello.[2Jworld") {
		t.Fatalf("unexpected preview: %q", preview)
	}
}

func TestRenderPreviewHexDumpsBinary(t *testing.T) {
	preview, binary := renderPreview([]byte{0x00, 0x01, 0xff, 0x10})
	if !binary {
		t.Fatal("binary preview not marked as binary")
	}
	if !strings.Contains(preview, "00000000") || strings.Contains(preview, "\x00") {
		t.Fatalf("unexpected binary preview: %q", preview)
	}
}
