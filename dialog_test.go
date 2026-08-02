package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func viewerTestModel(t *testing.T, path string) model {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "one.txt"))
	m := newModel(dir, dir, cfg{})
	m.width, m.height = 80, 24
	v, err := newViewer(path)
	if err != nil {
		t.Fatal(err)
	}
	m.dlg = newViewerDialog(v)
	return m
}

func TestViewerWKeyTogglesWrapAndTitleTag(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "long.txt"), strings.Repeat("word ", 40)+"\n")
	m := viewerTestModel(t, f)

	if m.dlg.viewer.wrap {
		t.Fatal("viewer should not start wrapped")
	}
	if strings.Contains(m.dlg.title, "[wrap]") {
		t.Fatalf("title has [wrap] before toggling: %q", m.dlg.title)
	}

	m, _ = m.handleDlgKey(key('w'))
	if !m.dlg.viewer.wrap {
		t.Error("'w' did not enable wrap")
	}
	if !strings.Contains(m.dlg.title, "[wrap]") {
		t.Errorf("title = %q, want it to contain [wrap]", m.dlg.title)
	}

	m, _ = m.handleDlgKey(key('w'))
	if m.dlg.viewer.wrap {
		t.Error("second 'w' did not disable wrap")
	}
	if strings.Contains(m.dlg.title, "[wrap]") {
		t.Errorf("title = %q, want [wrap] removed", m.dlg.title)
	}
}

func TestViewerHKeyStillTogglesHexAlongsideWrap(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "a.txt"), "hello\n")
	m := viewerTestModel(t, f)

	m, _ = m.handleDlgKey(key('w'))
	m, _ = m.handleDlgKey(key('h'))
	if !m.dlg.viewer.hex {
		t.Error("'h' did not enable hex mode")
	}
	if !m.dlg.viewer.wrap {
		t.Error("toggling hex should not clear a previously enabled wrap")
	}
	if !strings.Contains(m.dlg.title, "[hex]") || !strings.Contains(m.dlg.title, "[wrap]") {
		t.Errorf("title = %q, want both [hex] and [wrap]", m.dlg.title)
	}
}

func TestViewerScrollClampsToWrappedRowCount(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "long.txt"), strings.Repeat("word ", 200)+"\n")
	m := viewerTestModel(t, f)
	m, _ = m.handleDlgKey(key('w'))

	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyEnd})
	want := len(m.dlg.viewer.rows(m.width-1)) - 1
	if m.dlg.sel != want {
		t.Errorf("after End, sel = %d, want %d (last wrapped row)", m.dlg.sel, want)
	}
}

func TestViewerFrameWidthWithWrapAndMarkdown(t *testing.T) {
	withColor(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "r.md"), "# "+strings.Repeat("word ", 30)+"\nplain **bold** text\n")
	for _, w := range []int{40, 41, 79, 80, 120} {
		for _, h := range []int{6, 24} {
			m := viewerTestModel(t, f)
			m.width, m.height = w, h
			m, _ = m.handleDlgKey(key('w'))
			checkFrame(t, m.View(), w, h)
		}
	}
}
