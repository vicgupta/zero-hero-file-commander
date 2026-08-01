package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPanelLoadSortSelect(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "zebra.txt"))
	mustWrite(t, filepath.Join(dir, "alpha.txt"))
	mustWrite(t, filepath.Join(dir, "a2.go"))
	mustWrite(t, filepath.Join(dir, "a10.go"))
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)

	p := newPanel(dir, true)
	if p.entries[0].name != ".." {
		t.Fatalf("first entry should be .., got %q", p.entries[0].name)
	}
	// dirs first, then natural order (dotfiles sort before letters)
	want := []string{"sub", ".hidden", "a2.go", "a10.go", "alpha.txt", "zebra.txt"}
	names := make([]string, 0, len(want))
	for _, e := range p.entries[1:] {
		names = append(names, e.name)
	}
	if len(names) != len(want) {
		t.Fatalf("got %d entries, want %d", len(names), len(want))
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("entry %d = %q, want %q", i, names[i], want[i])
		}
	}

	// selection
	p.setCursor(1)
	p.toggleSel()
	if p.selCount() != 1 {
		t.Fatalf("selCount = %d, want 1", p.selCount())
	}
	p.invertSel()
	if p.selCount() != 5 {
		t.Fatalf("after invert selCount = %d, want 5", p.selCount())
	}
	p.clearSel()
	p.selectMask("*.go")
	if p.selCount() != 2 {
		t.Fatalf("after selectMask(*.go) selCount = %d, want 2", p.selCount())
	}

	// hidden files
	p.hideHidden = true
	p.reload()
	for _, e := range p.entries {
		if e.name == ".hidden" {
			t.Fatal("hidden file should be filtered when hideHidden")
		}
	}

	// enter subdirectory
	p.setCursor(0) // ".."
	if !p.enter() {
		t.Fatal("expected .. to be a directory")
	}
	if p.path != filepath.Dir(dir) {
		t.Fatalf("after .. path = %q, want %q", p.path, filepath.Dir(dir))
	}
}

func TestSelectedEntriesFallsBackToCursor(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "one.txt"))
	p := newPanel(dir, true)
	p.setCursor(1) // one.txt
	items := p.selectedEntries()
	if len(items) != 1 || items[0].name != "one.txt" {
		t.Fatalf("fallback selection = %+v, want [one.txt]", items)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHiddenPreferenceAppliesToTheFirstListing(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".secret"))
	mustWrite(t, filepath.Join(dir, "visible.txt"))

	// The opening listing must already honour the setting: the old code applied
	// hideHidden only after newPanel had read the directory.
	if p := newPanel(dir, false); p.has(".secret") {
		t.Error("hidden file listed when hidden files are off")
	}
	if p := newPanel(dir, true); !p.has(".secret") {
		t.Error("hidden file missing when hidden files are on")
	}
}

func TestModelHonoursHiddenConfigAtStartup(t *testing.T) {
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".secret"))
	m := newModel(dir, dir, cfg{Hidden: false})
	for _, p := range m.panels {
		if p.has(".secret") {
			t.Error("startup listing ignored the hidden-files setting")
		}
	}
}

func TestSymlinkedDirectoryIsListedAndEnterable(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "realdir")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(real, "inside.txt"))
	link := filepath.Join(dir, "linkdir")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// A broken link must not be mistaken for a directory.
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "broken")); err != nil {
		t.Fatal(err)
	}

	p := newPanel(dir, true)
	for _, e := range p.entries {
		switch e.name {
		case "linkdir":
			if !e.dir {
				t.Error("symlink to a directory should be listed as a directory")
			}
		case "broken":
			if e.dir {
				t.Error("broken symlink should not be listed as a directory")
			}
		}
	}
	for i, e := range p.entries {
		if e.name == "linkdir" {
			p.setCursor(i)
		}
	}
	if !p.enter() {
		t.Fatal("enter() refused a symlinked directory")
	}
	if !p.has("inside.txt") {
		t.Errorf("did not enter the link target; path = %s", p.path)
	}
}

func TestSelectionIsDroppedWhenLeavingADirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Same name in both directories: the tag must not follow us down.
	mustWrite(t, filepath.Join(dir, "foo.txt"))
	mustWrite(t, filepath.Join(sub, "foo.txt"))

	p := newPanel(dir, true)
	for i, e := range p.entries {
		if e.name == "foo.txt" {
			p.setCursor(i)
		}
	}
	p.toggleSel()
	if p.selCount() != 1 {
		t.Fatalf("setup: selCount = %d, want 1", p.selCount())
	}
	for i, e := range p.entries {
		if e.name == "sub" {
			p.setCursor(i)
		}
	}
	p.enter()
	if p.selCount() != 0 {
		t.Errorf("selection leaked into %s: %v", p.path, p.selected)
	}
	// With nothing tagged, operations fall back to the cursor row, which after
	// entering is "..", i.e. nothing — not the inner foo.txt.
	if items := p.selectedEntries(); len(items) != 0 {
		t.Errorf("operations would target %v in the new directory", items)
	}
	p.parent()
	if p.selCount() != 0 {
		t.Errorf("selection leaked back into the parent: %v", p.selected)
	}
}

func TestDirectoriesCarryModTime(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := newPanel(dir, true)
	for _, e := range p.entries {
		if e.name == "sub" {
			if e.mod.IsZero() {
				t.Fatal("directory has no mod time, so it renders a blank date and sorts wrongly")
			}
			if got := formatEntry(e, 44); !strings.Contains(got, e.mod.Format(dateLayout)) {
				t.Errorf("directory row lacks its date: %q", got)
			}
		}
	}
}

func TestSortByTimeOrdersDirectories(t *testing.T) {
	dir := t.TempDir()
	old, recent := filepath.Join(dir, "old"), filepath.Join(dir, "recent")
	for _, d := range []string{old, recent} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	p := newPanel(dir, true)
	p.sortMode = "time"
	p.reload()
	var dirs []string
	for _, e := range p.entries {
		if e.dir && e.name != ".." {
			dirs = append(dirs, e.name)
		}
	}
	if len(dirs) != 2 || dirs[0] != "recent" {
		t.Errorf("sort by time = %v, want newest first [recent old]", dirs)
	}
}
