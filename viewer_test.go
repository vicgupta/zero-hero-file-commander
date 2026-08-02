package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestViewerDetectsMarkdownByExtension(t *testing.T) {
	dir := t.TempDir()
	md := write(t, filepath.Join(dir, "notes.md"), "# hi\n")
	txt := write(t, filepath.Join(dir, "notes.txt"), "hi\n")

	v, err := newViewer(md)
	if err != nil {
		t.Fatal(err)
	}
	if !v.markdown {
		t.Error("newViewer(.md) did not set markdown")
	}

	v2, err := newViewer(txt)
	if err != nil {
		t.Fatal(err)
	}
	if v2.markdown {
		t.Error("newViewer(.txt) incorrectly set markdown")
	}
}

func TestViewerToggleWrapReflowsLongLines(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "long.txt"), strings.Repeat("word ", 40)+"\n")
	v, err := newViewer(f)
	if err != nil {
		t.Fatal(err)
	}
	unwrapped := v.rows(20)
	if len(unwrapped) != len(v.lines) {
		t.Fatalf("unwrapped row count = %d, want %d (one per source line)", len(unwrapped), len(v.lines))
	}
	v.toggleWrap()
	wrapped := v.rows(20)
	if len(wrapped) <= len(unwrapped) {
		t.Errorf("wrapping a long line did not increase row count: %d -> %d", len(unwrapped), len(wrapped))
	}
	for i, row := range wrapped {
		if got := row.width(); got > 20 {
			t.Errorf("wrapped row %d is %d cells wide, want <= 20: %q", i, got, row.plain())
		}
	}
}

func TestViewerRowsInvalidatesCacheOnToggle(t *testing.T) {
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "a.txt"), "hello\nworld\n")
	v, err := newViewer(f)
	if err != nil {
		t.Fatal(err)
	}
	before := v.rows(40)
	if before[0].plain() != "hello" {
		t.Fatalf("rows(40)[0] = %q, want %q", before[0].plain(), "hello")
	}
	v.toggleHex()
	after := v.rows(40)
	if after[0].plain() == before[0].plain() {
		t.Error("toggling hex mode did not invalidate the cached rows")
	}
	if !strings.Contains(after[0].plain(), "68 65 6c") { // "hel" in hex
		t.Errorf("hex rows = %q, want a hex dump", after[0].plain())
	}
}

func TestViewerRowsHighlightsMarkdown(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "r.md"), "# Title\nplain text\n")
	v, err := newViewer(f)
	if err != nil {
		t.Fatal(err)
	}
	rows := v.rows(80)
	if len(rows) < 1 || rows[0].plain() != "# Title" {
		t.Fatalf("rows[0].plain() = %q, want %q", rows[0].plain(), "# Title")
	}
	if rows[0][0].style.Render("x") != styleMdH1.Render("x") {
		t.Error("markdown viewer did not style the heading line as H1")
	}
}

func TestViewerRowsPlainTextIsNotHighlighted(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "r.txt"), "# not markdown\n")
	v, err := newViewer(f)
	if err != nil {
		t.Fatal(err)
	}
	rows := v.rows(80)
	if rows[0][0].style.Render("x") == styleMdH1.Render("x") {
		t.Error("a plain .txt file was given markdown heading styling")
	}
}
