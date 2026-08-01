package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// withColor forces styled output on, so the tests below see the ANSI escapes a
// real terminal gets. Without this the renderer emits plain text and every
// width bug hides.
func withColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// checkFrame asserts every row of a rendered frame occupies exactly w cells.
func checkFrame(t *testing.T, frame string, w, wantRows int) {
	t.Helper()
	rows := strings.Split(frame, "\n")
	if len(rows) != wantRows {
		t.Errorf("frame has %d rows, want %d", len(rows), wantRows)
	}
	for i, row := range rows {
		if got := lipgloss.Width(row); got != w {
			t.Errorf("row %d visible width = %d, want %d\n  %q", i, got, w, row)
		}
	}
}

// panelFixture builds a model with a directory, a plain file, a tagged file and
// a long name, so every styled branch of the renderer is exercised.
func panelFixture(t *testing.T) model {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	mustWrite(t, filepath.Join(dir, "one.txt"))
	mustWrite(t, filepath.Join(dir, "two.txt"))
	mustWrite(t, filepath.Join(dir, strings.Repeat("long-name-", 12)+".bin"))
	m := newModel(dir, dir, cfg{})
	m.width, m.height = 100, 30
	m.panels[0].selected["two.txt"] = true
	m.panels[0].setCursor(1)
	return m
}

func TestFrameWidthIsExactWhenStyled(t *testing.T) {
	withColor(t)
	m := panelFixture(t)
	for _, brief := range []bool{false, true} {
		m.brief = brief
		checkFrame(t, m.View(), m.width, m.height)
	}
}

// The bug that shipped: padding an already-styled string counts escape bytes as
// cells, so the active header collapsed to a few visible characters.
func TestStyledHeaderKeepsFullWidth(t *testing.T) {
	withColor(t)
	m := panelFixture(t)
	w := len([]rune(m.panels[0].path)) + 10
	rows := m.panels[0].render(w, 5, true, true)
	if got := rows[0].width(); got != w {
		t.Errorf("styled header width = %d, want %d", got, w)
	}
	if got := lipgloss.Width(rows[0].String()); got != w {
		t.Errorf("rendered header width = %d, want %d", got, w)
	}
	if !strings.Contains(rows[0].plain(), m.panels[0].path) {
		t.Errorf("header lost the path: %q", rows[0].plain())
	}
}

func TestFrameWidthWithDialogs(t *testing.T) {
	withColor(t)
	for _, tc := range []struct {
		name string
		open func(m *model)
	}{
		{"mkdir", func(m *model) { m.dlg = newInputDialog("Make directory", "New directory:", "", "mkdir") }},
		{"confirm", func(m *model) { m.dlg = newConfirmDialog("Delete 2 item(s)?", "delete", true) }},
		{"menu", func(m *model) { m.dlg = newMenuDialog("Commands", commandMenuItems()) }},
		{"help", func(m *model) { m.dlg = newHelpDialog() }},
		{"conflict", func(m *model) {
			m.dlg = newConflictDialog(&conflict{dst: "/tmp/x.txt", dstSize: 10, srcSize: 20})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := panelFixture(t)
			tc.open(&m)
			checkFrame(t, m.View(), m.width, m.height)
		})
	}
}

func TestFrameWidthViewer(t *testing.T) {
	withColor(t)
	m := panelFixture(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "v.txt")
	os.WriteFile(f, []byte(strings.Repeat("wide line of text ", 30)+"\nshort\n"), 0o644)
	v, err := newViewer(f)
	if err != nil {
		t.Fatal(err)
	}
	m.dlg = newViewerDialog(v)
	checkFrame(t, m.View(), m.width, m.height)
}

// Narrow and odd widths are where off-by-one padding shows up.
func TestFrameWidthAcrossTerminalSizes(t *testing.T) {
	withColor(t)
	for _, w := range []int{40, 41, 79, 80, 81, 120, 201} {
		for _, h := range []int{6, 7, 24, 25, 50} {
			m := panelFixture(t)
			m.width, m.height = w, h
			checkFrame(t, m.View(), w, h)
		}
	}
}

func TestFrameWidthWithQuickViewActive(t *testing.T) {
	withColor(t)
	for _, w := range []int{40, 41, 79, 80, 81, 120} {
		for _, h := range []int{6, 7, 24, 25} {
			m := panelFixture(t)
			m.width, m.height = w, h
			m.panels[0].setCursor(1) // land on a real file, not ".."
			m.panels[1].quickView = true
			checkFrame(t, m.View(), w, h)
		}
	}
}

func TestFullModeColumnsAlign(t *testing.T) {
	rows := []entry{
		{name: "a.go", size: 892},
		{name: "averyveryverylongfilename.go", size: 12 << 10},
		{name: "sub", dir: true},
	}
	for _, e := range rows {
		got := formatEntry(e, 44)
		if n := len([]rune(got)); n != 44 {
			t.Errorf("formatEntry(%q) width = %d, want 44: %q", e.name, n, got)
		}
	}
	// The size column must start at the same cell on every row, whatever the
	// name length (the truncation ellipsis is multi-byte, so count runes).
	col := func(e entry, sub string) int {
		row := formatEntry(e, 44)
		return len([]rune(row[:strings.Index(row, sub)]))
	}
	a, b, c := col(rows[0], "892"), col(rows[1], "12K"), col(rows[2], "<DIR>")
	if a != b || b != c+2 { // <DIR> is 5 wide, the others 3, all right-aligned
		t.Errorf("size column not aligned: 892@%d 12K@%d <DIR>@%d\n  %q\n  %q\n  %q",
			a, b, c, formatEntry(rows[0], 44), formatEntry(rows[1], 44), formatEntry(rows[2], 44))
	}
}

func TestSplitAndOverlayPreserveWidth(t *testing.T) {
	base := srow{{"aaaa", styleDefault}, {"bbbb", styleCursor}, {"cccc", styleSelected}}
	for x := 0; x <= 12; x++ {
		head, tail := base.splitAt(x)
		if head.width() != x || head.width()+tail.width() != 12 {
			t.Errorf("splitAt(%d): head=%d tail=%d", x, head.width(), tail.width())
		}
	}
	sub := srow{{"XX", styleDialog}}
	for x := 0; x <= 10; x++ {
		got := base.overlay(x, sub)
		if got.width() != 12 {
			t.Errorf("overlay at %d changed width to %d", x, got.width())
		}
		if want := strings.Repeat("a", 4) + "bbbbcccc"; got.plain()[x:x+2] != "XX" {
			t.Errorf("overlay at %d: %q (base %q)", x, got.plain(), want)
		}
	}
}

// A long path must not squeeze the typed command off the row.
func TestCommandLineStaysUsableWithADeepPath(t *testing.T) {
	withColor(t)
	m := panelFixture(t)
	m.panels[m.active].path = "/" + strings.Repeat("very-deep-directory/", 12)
	m.cmdFocus = true
	for _, r := range "du -sh" {
		m.cmdline.insert([]rune{r})
	}
	row := m.cmdlineRow(m.width)
	if got := row.width(); got != m.width {
		t.Errorf("row width = %d, want %d", got, m.width)
	}
	plain := row.plain()
	if !strings.Contains(plain, "du -sh") {
		t.Errorf("typed command scrolled off the row: %q", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("clipped path should be marked with an ellipsis: %q", plain)
	}
	if !strings.Contains(plain, "▌") {
		t.Errorf("caret is not visible: %q", plain)
	}
}
