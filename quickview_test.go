package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestQuickViewLinesShowsFileContent(t *testing.T) {
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "note.txt"), "hello\nworld\n")
	lines := quickViewLines(entry{name: "note.txt"}, f)
	if len(lines) < 2 || lines[0] != "hello" || lines[1] != "world" {
		t.Errorf("lines = %v, want [hello world ...]", lines)
	}
}

func TestQuickViewLinesPlaceholdersForInvalidTargets(t *testing.T) {
	cases := []struct {
		name string
		e    entry
		path string
		want string
	}{
		{"empty cursor", entry{}, "", "no file under the cursor"},
		{"dotdot", entry{name: ".."}, "/tmp", "no file under the cursor"},
		{"directory", entry{name: "sub", dir: true}, "/tmp/sub", "is a directory"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := quickViewLines(c.e, c.path)
			if len(lines) != 1 || !strings.Contains(lines[0], c.want) {
				t.Errorf("lines = %v, want a single line containing %q", lines, c.want)
			}
		})
	}
}

func TestQuickViewLinesReportsUnreadableFile(t *testing.T) {
	lines := quickViewLines(entry{name: "gone.txt"}, filepath.Join(t.TempDir(), "gone.txt"))
	if len(lines) != 1 || !strings.Contains(lines[0], "cannot read") {
		t.Errorf("lines = %v, want a 'cannot read' placeholder", lines)
	}
}

func TestRenderQuickViewIsExactlyWAndH(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "note.txt"), strings.Repeat("x", 200)+"\nshort\n")
	for _, active := range []bool{true, false} {
		rows := renderQuickView(entry{name: "note.txt"}, f, 40, 8, active)
		if len(rows) != 8 {
			t.Fatalf("got %d rows, want 8", len(rows))
		}
		for i, r := range rows {
			if got := r.width(); got != 40 {
				t.Errorf("row %d width = %d, want 40", i, got)
			}
		}
	}
}

func TestRenderQuickViewHeaderNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "report.txt"), "x")
	rows := renderQuickView(entry{name: "report.txt"}, f, 60, 4, true)
	if !strings.Contains(rows[0].plain(), "report.txt") {
		t.Errorf("header = %q, want it to name the file", rows[0].plain())
	}
}

func TestQuickViewHandlesAnEmptyDirectory(t *testing.T) {
	// Regression guard: an empty *other* panel means srcEntry is the zero
	// value, and filepath.Join(dir, "") must not be treated as "view the
	// directory itself" — it should hit the same placeholder as no cursor.
	rows := renderQuickView(entry{}, filepath.Join(t.TempDir(), ""), 40, 4, true)
	if !strings.Contains(rows[1].plain(), "no file under the cursor") {
		t.Errorf("row 1 = %q, want the no-file placeholder", rows[1].plain())
	}
}
