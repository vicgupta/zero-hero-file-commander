package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// entry is one row in a panel listing.
type entry struct {
	name string
	dir  bool
	size int64
	mod  time.Time
	mode os.FileMode
}

// panel is the state of one side of the dual-pane interface.
type panel struct {
	path       string
	entries    []entry
	cursor     int
	scroll     int
	selected   map[string]bool
	sortMode   string // "name" | "size" | "time"
	sortStep   int    // Shift+S cycle position: 0=name 1=size 2=time 3=time+hide-dotfiles 4=time+show-dotfiles
	hideHidden bool
	search     string // accumulated fast-search prefix, original typed case
	quickView  bool   // showing a live preview of the other panel's cursor file, not our own listing
}

func newPanel(path string, showHidden bool) *panel {
	if path == "" || path == "~" {
		path, _ = os.UserHomeDir()
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	// hideHidden must be set before the first read, or the opening listing
	// ignores the user's configured preference.
	p := &panel{selected: map[string]bool{}, sortMode: "name", hideHidden: !showHidden}
	p.path = path
	p.reload()
	return p
}

func (p *panel) reload() {
	entries, err := os.ReadDir(p.path)
	ne := make([]entry, 0, len(entries)+1)
	if p.path != "/" {
		ne = append(ne, entry{name: "..", dir: true})
	}
	if err == nil {
		for _, de := range entries {
			if p.hideHidden && strings.HasPrefix(de.Name(), ".") {
				continue
			}
			en := entry{name: de.Name(), dir: de.IsDir()}
			fi, ferr := de.Info() // lstat: describes a symlink, not its target
			if de.Type()&os.ModeSymlink != 0 {
				// Resolve links so one pointing at a directory lists and
				// behaves as a directory. Broken links stay files.
				if st, serr := os.Stat(filepath.Join(p.path, de.Name())); serr == nil {
					en.dir = st.IsDir()
					fi, ferr = st, nil
				}
			}
			if ferr == nil {
				en.mod = fi.ModTime()
				en.mode = fi.Mode()
				if !en.dir {
					en.size = fi.Size()
				}
			}
			ne = append(ne, en)
		}
	}
	p.entries = ne
	p.sort()
	for name := range p.selected {
		if !p.has(name) {
			delete(p.selected, name)
		}
	}
	p.clamp()
}

func (p *panel) has(name string) bool {
	for _, e := range p.entries {
		if e.name == name {
			return true
		}
	}
	return false
}

func (p *panel) sort() {
	sort.SliceStable(p.entries, func(i, j int) bool {
		a, b := p.entries[i], p.entries[j]
		if a.name == ".." {
			return true
		}
		if b.name == ".." {
			return false
		}
		if a.dir != b.dir {
			return a.dir
		}
		switch p.sortMode {
		case "time":
			if !a.mod.Equal(b.mod) {
				return a.mod.After(b.mod)
			}
		case "size":
			if a.size != b.size {
				return a.size > b.size
			}
		}
		return naturalLess(a.name, b.name)
	})
}

func (p *panel) clamp() {
	if len(p.entries) == 0 {
		p.cursor, p.scroll = 0, 0
		return
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
}

// ensureVisible keeps the cursor row within a viewport of h rows.
func (p *panel) ensureVisible(h int) {
	if h <= 0 {
		return
	}
	if p.cursor < p.scroll {
		p.scroll = p.cursor
	}
	if p.cursor >= p.scroll+h {
		p.scroll = p.cursor - h + 1
	}
}

func (p *panel) move(d int) {
	p.search = ""
	p.cursor += d
	p.clamp()
}

func (p *panel) setCursor(i int) {
	p.search = ""
	p.cursor = i
	p.clamp()
}

// setPath moves the panel to dir, dropping the cursor, scroll offset,
// selection and any in-progress fast search — all of which belong to the
// directory being left. Without the selection reset a tagged name stays
// tagged in the new directory if a file there shares it, and the next F8
// deletes the wrong file.
func (p *panel) setPath(dir string) {
	p.path = dir
	p.cursor = 0
	p.scroll = 0
	p.search = ""
	p.clearSel()
	p.reload()
}

// searchStep extends the fast-search prefix by r and jumps the cursor to the
// first entry whose name starts with it, case-insensitively. It reports
// whether a match was found; on failure the panel's search state and cursor
// are left untouched, so the caller can decide how to handle the miss.
func (p *panel) searchStep(r rune) bool {
	candidate := p.search + string(r)
	idx := p.findSearchMatch(candidate)
	if idx < 0 {
		return false
	}
	p.search = candidate
	p.cursor = idx
	p.clamp()
	return true
}

// findSearchMatch returns the index of the first non-".." entry whose name
// has prefix as a case-insensitive prefix, or -1 if none matches.
func (p *panel) findSearchMatch(prefix string) int {
	lower := strings.ToLower(prefix)
	for i, e := range p.entries {
		if e.name == ".." {
			continue
		}
		if strings.HasPrefix(strings.ToLower(e.name), lower) {
			return i
		}
	}
	return -1
}

// resetSearch abandons any in-progress fast search without moving the cursor.
func (p *panel) resetSearch() {
	p.search = ""
}

func (p *panel) parent() {
	p.setPath(filepath.Dir(p.path))
}

// enter descends into the directory under the cursor. It reports whether the
// cursor was on a directory (as opposed to a file).
func (p *panel) enter() bool {
	if len(p.entries) == 0 {
		return false
	}
	e := p.entries[p.cursor]
	if !e.dir {
		return false
	}
	if e.name == ".." {
		p.parent()
		return true
	}
	np := filepath.Join(p.path, e.name)
	if st, err := os.Stat(np); err == nil && st.IsDir() {
		p.setPath(np)
	}
	return true
}

func (p *panel) toggleSel() {
	if len(p.entries) == 0 {
		return
	}
	e := p.entries[p.cursor]
	if e.name == ".." {
		return
	}
	if p.selected[e.name] {
		delete(p.selected, e.name)
	} else {
		p.selected[e.name] = true
	}
	if p.cursor+1 < len(p.entries) {
		p.cursor++
	}
}

func (p *panel) invertSel() {
	for _, e := range p.entries {
		if e.name == ".." {
			continue
		}
		if p.selected[e.name] {
			delete(p.selected, e.name)
		} else {
			p.selected[e.name] = true
		}
	}
}

func (p *panel) clearSel() {
	p.selected = map[string]bool{}
}

func (p *panel) selectMask(mask string) {
	for _, e := range p.entries {
		if e.name != ".." && matchMask(e.name, mask) {
			p.selected[e.name] = true
		}
	}
}

func (p *panel) unselectMask(mask string) {
	for _, e := range p.entries {
		if e.name != ".." && matchMask(e.name, mask) {
			delete(p.selected, e.name)
		}
	}
}

// selectedEntries returns the files to operate on: the current selection if
// non-empty, otherwise the file under the cursor.
func (p *panel) selectedEntries() []entry {
	var out []entry
	for _, e := range p.entries {
		if e.name != ".." && p.selected[e.name] {
			out = append(out, e)
		}
	}
	if len(out) == 0 && len(p.entries) > 0 {
		e := p.entries[p.cursor]
		if e.name != ".." {
			out = append(out, e)
		}
	}
	return out
}

func (p *panel) selCount() int {
	n := 0
	for _, e := range p.entries {
		if e.name != ".." && p.selected[e.name] {
			n++
		}
	}
	return n
}

func (p *panel) selSize() int64 {
	var n int64
	for _, e := range p.entries {
		if e.name != ".." && p.selected[e.name] {
			n += e.size
		}
	}
	return n
}

// visibleEntries returns the entries for the current viewport.
func (p *panel) visibleEntries(h int) []entry {
	if h <= 0 {
		return nil
	}
	p.ensureVisible(h)
	hi := p.scroll + h
	if hi > len(p.entries) {
		hi = len(p.entries)
	}
	return p.entries[p.scroll:hi]
}

// render draws the panel as h rows, each exactly w cells wide.
func (p *panel) render(w, h int, active, brief bool) []srow {
	rows := make([]srow, h)
	hdr := styleInactiveHeader
	if active {
		hdr = styleActiveHeader
	}
	rows[0] = srow{{padRune(" "+p.path, w), hdr}}

	vis := p.visibleEntries(h - 1)
	for i, e := range vis {
		var text string
		switch {
		case e.name == "..":
			text = padRune(" ..", w)
		case brief:
			text = padRune(" "+truncRune(e.name, w-1), w)
		default:
			text = formatEntry(e, w)
		}
		sel := e.name != ".." && p.selected[e.name]
		st := styleDefault
		switch {
		case p.scroll+i == p.cursor && sel:
			st = styleCursorSelected
		case p.scroll+i == p.cursor && e.dir:
			st = styleCursorDir
		case p.scroll+i == p.cursor:
			st = styleCursor
		case sel:
			st = styleSelected
		case e.dir:
			st = styleDir
		}
		rows[i+1] = srow{{text, st}}
	}
	for i := len(vis) + 1; i < h; i++ {
		rows[i] = srow{{strings.Repeat(" ", w), styleDefault}}
	}
	return rows
}

// formatEntry lays out one row in Full display mode, always exactly w cells:
// name padded to a fixed column, then a right-aligned size and the date.
func formatEntry(e entry, w int) string {
	if e.name == ".." {
		return padRune(" ..", w)
	}
	size := humanSize(e.size)
	if e.dir {
		size = "<DIR>"
	}
	// Directories are not stat'd during listing, so their mtime is zero; show
	// blanks rather than a fabricated 01-01-01.
	date := strings.Repeat(" ", len(dateLayout))
	if !e.mod.IsZero() {
		date = e.mod.Format(dateLayout)
	}
	right := " " + padLeft(size, 8) + " " + date // ASCII only, so len == cells
	nameW := w - len(right) - 1
	if nameW < 3 {
		return padRune(" "+truncRune(e.name, w-1), w)
	}
	return padRune(" "+padRune(truncRune(e.name, nameW), nameW)+right, w)
}

const dateLayout = "01-02-06 15:04"
