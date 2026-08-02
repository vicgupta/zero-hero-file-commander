package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestIsMarkdownPath(t *testing.T) {
	cases := map[string]bool{
		"README.md":       true,
		"notes.MARKDOWN":  true,
		"x.mdown":         true,
		"main.go":         false,
		"noextension":     false,
		"dir/README.md":   true,
		"weird.md.backup": false,
	}
	for path, want := range cases {
		if got := isMarkdownPath(path); got != want {
			t.Errorf("isMarkdownPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestMdHighlightLineHeadingLevels(t *testing.T) {
	withColor(t)
	for _, tc := range []struct {
		line string
		want lipgloss.Style
	}{
		{"# Title", styleMdH1},
		{"## Subtitle", styleMdH2},
		{"### Section", styleMdH3},
	} {
		row := mdHighlightLine(tc.line)
		if len(row) == 0 {
			t.Fatalf("mdHighlightLine(%q) produced no spans", tc.line)
		}
		if row[0].style.Render("x") != tc.want.Render("x") {
			t.Errorf("mdHighlightLine(%q): first span style = %v, want the heading style", tc.line, row[0].style)
		}
		if !strings.Contains(row.plain(), strings.TrimLeft(tc.line, "# ")) {
			t.Errorf("mdHighlightLine(%q) lost the heading text: %q", tc.line, row.plain())
		}
	}
}

func TestMdHighlightLinePreservesTextThroughInlineStyling(t *testing.T) {
	line := "plain **bold** plain *italic* plain `code` plain [link](http://x) end"
	row := mdHighlightLine(line)
	plain := row.plain()
	for _, want := range []string{"plain", "bold", "italic", "code", "link", "end"} {
		if !strings.Contains(plain, want) {
			t.Errorf("highlighted line lost text %q: %q", want, plain)
		}
	}
	// The URL should still show up somewhere, so following a link doesn't
	// require leaving the viewer.
	if !strings.Contains(plain, "http://x") {
		t.Errorf("highlighted line dropped the link URL: %q", plain)
	}
}

func TestMdHighlightLineListAndQuoteMarkers(t *testing.T) {
	withColor(t)
	if row := mdHighlightLine("- an item"); !strings.Contains(row.plain(), "an item") {
		t.Errorf("list item lost text: %q", row.plain())
	}
	if row := mdHighlightLine("> a quote"); !strings.Contains(row.plain(), "a quote") {
		t.Errorf("blockquote lost text: %q", row.plain())
	}
}

func TestMdHighlightLinesTracksFencedCodeBlocks(t *testing.T) {
	withColor(t)
	lines := []string{
		"# Heading",
		"```",
		"# not a heading inside a fence",
		"```",
		"# Heading again",
	}
	rows := mdHighlightLines(lines)
	if len(rows) != len(lines) {
		t.Fatalf("got %d rows, want %d", len(rows), len(lines))
	}
	// Inside the fence, a line that looks like a heading must NOT get heading
	// styling — it's code, not markdown structure.
	fenced := rows[2]
	if len(fenced) > 0 && fenced[0].style.Render("x") == styleMdH1.Render("x") {
		t.Errorf("line inside a fence was styled as a heading: %q", fenced.plain())
	}
	if rows[0][0].style.Render("x") != styleMdH1.Render("x") {
		t.Errorf("heading before the fence lost its styling")
	}
	if rows[4][0].style.Render("x") != styleMdH1.Render("x") {
		t.Errorf("heading after the fence lost its styling — fence state leaked")
	}
}
