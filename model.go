package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type cmdline struct {
	text []rune
	cur  int
	hist []string
	hIdx int
}

func (c *cmdline) insert(r []rune) {
	head := append([]rune{}, c.text[:c.cur]...)
	tail := c.text[c.cur:]
	c.text = append(head, r...)
	c.text = append(c.text, tail...)
	c.cur += len(r)
}

func (c *cmdline) delLeft() {
	if c.cur > 0 {
		c.text = append(c.text[:c.cur-1], c.text[c.cur:]...)
		c.cur--
	}
}

func (c *cmdline) clear() {
	c.text = c.text[:0]
	c.cur = 0
}

func (c *cmdline) pushHist(s string) {
	if len(c.hist) == 0 || c.hist[len(c.hist)-1] != s {
		c.hist = append(c.hist, s)
	}
	c.hIdx = len(c.hist)
}

// recall walks command history; dir +1 goes older, -1 newer.
func (c *cmdline) recall(dir int) {
	if len(c.hist) == 0 {
		return
	}
	if dir > 0 && c.hIdx > 0 {
		c.hIdx--
		c.text = []rune(c.hist[c.hIdx])
		c.cur = len(c.text)
	}
	if dir < 0 && c.hIdx < len(c.hist)-1 {
		c.hIdx++
		c.text = []rune(c.hist[c.hIdx])
		c.cur = len(c.text)
	}
}

type execDoneMsg struct{ err error }

type editDoneMsg struct {
	err  error
	path string
}

type model struct {
	width, height int
	panels        [2]*panel
	active        int
	cmdline       cmdline
	dlg           *dialog
	status        string
	statusErr     bool
	cmdFocus      bool // ':' pressed — send every key to the command line

	opCh     chan tea.Msg
	opCancel context.CancelFunc
	opRun    bool
	conflict *conflict // collision awaiting the user's answer

	cfg    cfg
	brief  bool
	hidden bool
}

// fail reports a problem on the status line without starting anything.
func (m model) fail(s string) (model, tea.Cmd) {
	m.status = s
	m.statusErr = true
	return m, nil
}

func newModel(startL, startR string, c cfg) model {
	theme := c.Theme
	if theme == "" {
		theme = defaultTheme
	}
	applyTheme(theme)
	m := model{
		opCh:   make(chan tea.Msg),
		cfg:    c,
		brief:  c.Brief,
		hidden: c.Hidden,
	}
	m.panels[0] = newPanel(startL, c.Hidden)
	m.panels[1] = newPanel(startR, c.Hidden)
	return m
}

// cmdIdle reports whether the command line is dormant, so bare letters act as
// navigation instead of input. Typing anything, or pressing ':', wakes it.
func (m model) cmdIdle() bool {
	return m.cfg.ViKeys && !m.cmdFocus && len(m.cmdline.text) == 0
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		var cmd tea.Cmd
		m, cmd = m.handleKey(msg)
		return m, cmd
	case opMsg:
		m.status = msg.label
		m.statusErr = msg.err != nil
		if msg.done {
			m.opRun = false
			m.opCancel = nil
			// Reload even after a failure: a partial copy still changed things.
			m.panels[0].reload()
			m.panels[1].reload()
			return m, nil
		}
		return m, m.opSub()
	case conflictMsg:
		// The operation goroutine is parked until answerConflict replies, so
		// deliberately do not re-subscribe here.
		m.conflict = msg.c
		m.dlg = newConflictDialog(msg.c)
		return m, nil
	case execDoneMsg:
		m.panels[0].reload()
		m.panels[1].reload()
		if msg.err != nil {
			m.status = "command failed: " + msg.err.Error()
		} else {
			m.status = ""
		}
	case editDoneMsg:
		m.panels[0].reload()
		m.panels[1].reload()
		if msg.err != nil {
			m.status = "editor failed: " + msg.err.Error()
			m.statusErr = true
		}
	}
	// No blanket re-subscribe: exactly one opSub() is outstanding at a time, so
	// stray messages here must not spawn another receiver (it would block on
	// opCh forever once the operation ends).
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlQ {
		return m, m.quitCmd()
	}
	// Dialogs come first: a conflict prompt is open while an operation runs.
	if m.dlg != nil {
		return m.handleDlgKey(msg)
	}
	if m.opRun {
		if msg.Type == tea.KeyEsc && m.opCancel != nil {
			m.opCancel()
		}
		return m, nil
	}
	return m.handlePanelKey(msg)
}

// answerConflict releases the parked operation and resumes listening to it.
func (m model) answerConflict(a conflictAnswer) (model, tea.Cmd) {
	c := m.conflict
	m.conflict = nil
	m.dlg = nil
	if c == nil {
		return m, nil
	}
	c.reply <- a // buffered, never blocks
	return m, m.opSub()
}

func (m model) quitCmd() tea.Cmd {
	if m.opCancel != nil {
		m.opCancel()
	}
	m.cfg.Brief = m.brief
	m.cfg.Hidden = m.hidden
	m.cfg.Theme = activeTheme
	m.cfg.save()
	return tea.Quit
}

func (m model) handlePanelKey(msg tea.KeyMsg) (model, tea.Cmd) {
	p := m.panels[m.active]
	switch msg.Type {
	case tea.KeyF1:
		m.dlg = newHelpDialog()
	case tea.KeyF2:
		m.dlg = newMenuDialog("User menu", userMenuItems())
	case tea.KeyF3:
		return m.openViewer()
	case tea.KeyF4:
		return m.openEditor()
	case tea.KeyF5:
		return m.copyDialog()
	case tea.KeyF6:
		return m.moveDialog()
	case tea.KeyF7:
		m.dlg = newInputDialog("Make directory", "New directory:", "", "mkdir")
	case tea.KeyF8:
		return m.deleteConfirm()
	case tea.KeyF9:
		m.dlg = newMenuDialog("Commands", commandMenuItems())
	case tea.KeyF10:
		return m, m.quitCmd()
	case tea.KeyF11:
		m.dlg = newInputDialog("Change attributes", "Mode (octal, e.g. 644):", "644", "chmod")
	case tea.KeyTab, tea.KeyShiftTab:
		m.active = 1 - m.active
	case tea.KeyUp, tea.KeyShiftUp:
		p.move(-1)
	case tea.KeyDown, tea.KeyShiftDown:
		p.move(1)
	case tea.KeyPgUp:
		p.move(-m.panelRows())
	case tea.KeyPgDown:
		p.move(m.panelRows())
	case tea.KeyHome:
		p.setCursor(0)
	case tea.KeyEnd:
		p.setCursor(len(p.entries) - 1)
	case tea.KeyLeft, tea.KeyShiftLeft:
		p.parent()
	case tea.KeyRight, tea.KeyShiftRight:
		p.enter()
	case tea.KeyEnter:
		if len(m.cmdline.text) > 0 {
			return m.runCommand(string(m.cmdline.text))
		}
		if !p.enter() {
			return m.openViewer()
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if len(m.cmdline.text) > 0 {
			m.cmdline.delLeft()
		}
	case tea.KeyEsc:
		m.cmdline.clear()
		m.cmdFocus = false
		p.clearSel()
		p.resetSearch()
	case tea.KeyInsert, tea.KeySpace:
		if len(m.cmdline.text) > 0 && msg.Type == tea.KeySpace {
			m.cmdline.insert(msg.Runes)
		} else {
			p.toggleSel()
		}
	case tea.KeyCtrlR:
		p.reload()
	case tea.KeyCtrlU:
		m.panels[0], m.panels[1] = m.panels[1], m.panels[0]
	case tea.KeyCtrlE:
		m.cmdline.recall(1)
	case tea.KeyCtrlA:
		p.selectMask("*")
	case tea.KeyCtrlT:
		p.invertSel()
	case tea.KeyCtrlD:
		p.clearSel()
	case tea.KeyCtrlB:
		m.brief = !m.brief
	case tea.KeyRunes:
		// vi-style navigation, but only while the command line is dormant —
		// otherwise "cat file" and friends could never be typed. ':' hands the
		// keyboard back to the command line so those commands stay reachable.
		if len(msg.Runes) == 1 && m.cmdIdle() {
			switch msg.Runes[0] {
			case 'j':
				p.move(1)
				return m, nil
			case 'k':
				p.move(-1)
				return m, nil
			case 'n':
				p.move(m.panelRows())
				return m, nil
			case 'u':
				p.move(-m.panelRows())
				return m, nil
			case 'h':
				p.parent()
				return m, nil
			case 'l':
				p.enter()
				return m, nil
			case 'v':
				if !p.enter() {
					return m.openViewer()
				}
				return m, nil
			case ':':
				m.cmdFocus = true
				return m, nil
			}
			// Fast search: build a filename prefix from what's typed and jump
			// to the first match. A character that breaks the match means this
			// was never a search — flush everything typed so far into the
			// command line instead, exactly as if fast search had never
			// intercepted it.
			r := msg.Runes[0]
			if p.searchStep(r) {
				return m, nil
			}
			typed := p.search + string(r)
			p.resetSearch()
			m.cmdline.insert([]rune(typed))
			return m, nil
		}
		m.cmdline.insert(msg.Runes)
	default:
		if len(msg.Runes) > 0 {
			m.cmdline.insert(msg.Runes)
		}
	}
	return m, nil
}

func (m model) runCommand(cmdStr string) (model, tea.Cmd) {
	cmdStr = strings.TrimSpace(cmdStr)
	m.cmdline.pushHist(cmdStr)
	m.cmdline.clear()
	m.cmdFocus = false
	p := m.panels[m.active]
	if cmdStr == ".." || cmdStr == "cd .." {
		p.parent()
		return m, nil
	}
	if strings.HasPrefix(cmdStr, "cd ") {
		dir := expandTilde(strings.TrimSpace(strings.TrimPrefix(cmdStr, "cd ")))
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(p.path, dir)
		}
		dir = filepath.Clean(dir)
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			p.setPath(dir)
		} else {
			m.status = "no such directory: " + dir
		}
		return m, nil
	}
	m.status = "Running: " + cmdStr
	return m, execShellCmd(p.path, cmdStr)
}

// execShellCmd runs a command in the shell with the panel directory as cwd.
func execShellCmd(dir, cmdStr string) tea.Cmd {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/C", cmdStr)
	} else {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		c = exec.Command(shell, "-c", cmdStr)
	}
	c.Dir = dir
	return tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err: err} })
}

func editCmd(path string) tea.Cmd {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		if runtime.GOOS == "windows" {
			ed = "notepad"
		} else {
			ed = "vi"
		}
	}
	parts := strings.Fields(ed)
	c := exec.Command(parts[0], append(parts[1:], path)...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editDoneMsg{err: err, path: path}
	})
}

func (m model) openViewer() (model, tea.Cmd) {
	p := m.panels[m.active]
	if len(p.entries) == 0 {
		return m, nil
	}
	e := p.entries[p.cursor]
	if e.name == ".." {
		p.parent()
		return m, nil
	}
	path := filepath.Join(p.path, e.name)
	if e.dir {
		p.enter()
		return m, nil
	}
	v, err := newViewer(path)
	if err != nil {
		m.status = "view: " + err.Error()
		return m, nil
	}
	m.dlg = newViewerDialog(v)
	return m, nil
}

func (m model) openEditor() (model, tea.Cmd) {
	p := m.panels[m.active]
	if len(p.entries) == 0 {
		return m, nil
	}
	e := p.entries[p.cursor]
	if e.name == ".." {
		return m, nil
	}
	if e.dir {
		m.status = "cannot edit a directory"
		return m, nil
	}
	return m, editCmd(filepath.Join(p.path, e.name))
}

func (m model) copyDialog() (model, tea.Cmd) {
	p := m.panels[m.active]
	if len(p.selectedEntries()) == 0 {
		m.status = "nothing to copy"
		return m, nil
	}
	m.dlg = newInputDialog("Copy", "Copy to:", m.panels[1-m.active].path, "copy")
	return m, nil
}

func (m model) moveDialog() (model, tea.Cmd) {
	p := m.panels[m.active]
	if len(p.selectedEntries()) == 0 {
		m.status = "nothing to move"
		return m, nil
	}
	m.dlg = newInputDialog("Move / Rename", "Move to:", m.panels[1-m.active].path, "move")
	return m, nil
}

func (m model) deleteConfirm() (model, tea.Cmd) {
	p := m.panels[m.active]
	items := p.selectedEntries()
	if len(items) == 0 {
		m.status = "nothing to delete"
		return m, nil
	}
	m.dlg = newConfirmDialog(fmt.Sprintf("Delete %d item(s)?", len(items)), "delete")
	return m, nil
}

func userMenuItems() []menuItem {
	return []menuItem{
		{"Select by mask...", "select_mask"},
		{"Unselect by mask...", "unselect_mask"},
		{"Invert selection", "invert"},
		{"Clear selection", "clear_sel"},
		{"New file...", "newfile"},
	}
}

func commandMenuItems() []menuItem {
	return []menuItem{
		{"Reread panels", "reread"},
		{"Swap panels", "swap"},
		{"Toggle brief/full display", "toggle_brief"},
		{"Toggle hidden files", "toggle_hidden"},
		{"Sort by name", "sort_name"},
		{"Sort by time", "sort_time"},
		{"View file under cursor", "view"},
		{"Edit file under cursor", "edit"},
		{"Theme...", "theme"},
		{"Quit", "quit"},
	}
}

// themeLabels are the display names for themeNames, in the same order.
var themeLabels = map[string]string{
	"norton":   "Norton (classic blue)",
	"nightowl": "Night Owl (navy blue)",
	"opencode": "opencode (dark amber)",
}

// themeMenuItems lists the built-in themes, marking the active one.
func themeMenuItems() []menuItem {
	items := make([]menuItem, len(themeNames))
	for i, name := range themeNames {
		label := themeLabels[name]
		if name == activeTheme {
			label += "  (current)"
		}
		items[i] = menuItem{label, "theme_" + name}
	}
	return items
}

// doAction executes an action that originated from a menu or dialog submit.
// val carries the dialog input value for input-driven actions.
func (m model) doAction(action, val string) (model, tea.Cmd) {
	p := m.panels[m.active]
	if name, ok := strings.CutPrefix(action, "theme_"); ok {
		applyTheme(name)
		m.status = "Theme: " + themeLabels[name]
		return m, nil
	}
	switch action {
	case "quit":
		return m, m.quitCmd()
	case "theme":
		m.dlg = newMenuDialog("Theme", themeMenuItems())
	case "reread":
		m.panels[0].reload()
		m.panels[1].reload()
	case "swap":
		m.panels[0], m.panels[1] = m.panels[1], m.panels[0]
	case "toggle_brief":
		m.brief = !m.brief
	case "toggle_hidden":
		m.hidden = !m.hidden
		m.panels[0].hideHidden = !m.hidden
		m.panels[1].hideHidden = !m.hidden
		m.panels[0].reload()
		m.panels[1].reload()
	case "sort_name":
		p.sortMode = "name"
		p.reload()
	case "sort_time":
		p.sortMode = "time"
		p.reload()
	case "view":
		return m.openViewer()
	case "edit":
		return m.openEditor()
	case "copy":
		dst := expandTilde(strings.TrimSpace(val))
		if dst == "" {
			dst = m.panels[1-m.active].path
		}
		return m.startCopy(p.path, dst, p.selectedEntries())
	case "move":
		dst := expandTilde(strings.TrimSpace(val))
		if dst == "" {
			dst = m.panels[1-m.active].path
		}
		return m.startMove(p.path, dst, p.selectedEntries())
	case "mkdir":
		name := strings.TrimSpace(val)
		if name == "" {
			return m, nil
		}
		if err := os.MkdirAll(filepath.Join(p.path, name), 0o755); err != nil {
			m.status = "mkdir: " + err.Error()
		} else {
			p.reload()
			m.status = "created " + name
		}
	case "delete":
		return m.startDelete(p.path, p.selectedEntries())
	case "chmod":
		mode, err := strconv.ParseUint(strings.TrimSpace(val), 8, 32)
		if err != nil {
			m.status = "bad mode"
			return m, nil
		}
		for _, it := range p.selectedEntries() {
			os.Chmod(filepath.Join(p.path, it.name), os.FileMode(mode))
		}
		m.status = "attributes changed"
	case "select_mask":
		p.selectMask(strings.TrimSpace(val))
	case "unselect_mask":
		p.unselectMask(strings.TrimSpace(val))
	case "invert":
		p.invertSel()
	case "clear_sel":
		p.clearSel()
	case "newfile":
		name := strings.TrimSpace(val)
		if name == "" {
			return m, nil
		}
		fp := filepath.Join(p.path, name)
		if _, err := os.Stat(fp); os.IsNotExist(err) {
			if err := os.WriteFile(fp, nil, 0o644); err != nil {
				m.status = "create: " + err.Error()
				return m, nil
			}
		}
		p.reload()
		return m, editCmd(fp)
	}
	return m, nil
}

// ---- View ----

// panelRows is the number of file rows a panel shows, excluding its header.
func (m model) panelRows() int {
	n := m.height - 3 // panel header + command line + key bar
	if n < 1 {
		n = 1
	}
	return n
}

func (m model) View() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return "" // first frame before WindowSizeMsg
	}
	if w < 40 || h < 6 {
		return fmt.Sprintf("terminal too small: %dx%d (minimum 40x6)", w, h)
	}
	pw := (w - 1) / 2
	ph := m.panelRows() + 1
	left := m.panels[0].render(pw, ph, m.active == 0, m.brief)
	right := m.panels[1].render(pw, ph, m.active == 1, m.brief)

	rows := make([]srow, 0, h)
	for i := 0; i < ph; i++ {
		r := make(srow, 0, len(left[i])+len(right[i])+2)
		r = append(r, left[i]...)
		r = append(r, span{"│", styleDivider})
		r = append(r, right[i]...)
		rows = append(rows, r.pad(w, styleDefault))
	}
	rows = append(rows, m.cmdlineRow(w))
	rows = append(rows, m.keybarRow(w))
	if m.dlg != nil {
		rows = m.dialogRows(rows)
	}
	return renderRows(rows)
}

func (m model) cmdlineRow(w int) srow {
	if m.status != "" {
		st := styleStatus
		if m.statusErr {
			st = styleError
		}
		return srow{{padRune(" "+m.status, w), st}}
	}
	p := m.panels[m.active]
	prompt := p.path + "> "
	if n := p.selCount(); n > 0 && len(m.cmdline.text) == 0 {
		prompt += fmt.Sprintf("(%d selected) ", n)
	}
	// A dim caret means bare j/k/d/u will navigate; a bright one means they
	// will be typed. Without the cue there is no way to tell the modes apart.
	caret := styleCmdCaret
	if m.cmdIdle() {
		caret = styleCmdCaretIdle
	}
	full := []rune(prompt + string(m.cmdline.text))
	cur := clamp(len([]rune(prompt))+m.cmdline.cur, 0, len(full))
	// Scroll the line so the caret is always on screen. A deep directory would
	// otherwise fill the row with prompt and leave nowhere to type.
	off := 0
	if cur+1 > w {
		off = cur + 1 - w
	}
	head := append([]rune{}, full[off:cur]...)
	if off > 0 && len(head) > 0 {
		head[0] = '…' // signal the path is clipped on the left
	}
	row := srow{
		{string(head), styleCmdline},
		{"▌", caret},
		{string(full[cur:]), styleCmdline},
	}
	return row.pad(w, styleCmdline)
}

var fkeys = []struct{ k, l string }{
	{"F1", "Help"}, {"F2", "Menu"}, {"F3", "View"}, {"F4", "Edit"},
	{"F5", "Copy"}, {"F6", "Move"}, {"F7", "MkDir"}, {"F8", "Del"},
	{"F9", "Menu"}, {"F10", "Quit"}, {"F11", "Chmod"},
}

// keybarRow draws the NC function-key bar: key number on black, label on cyan.
// The gap between key and label is dropped on narrow terminals so the whole bar
// still fits the spec's 80-column minimum.
func (m model) keybarRow(w int) srow {
	gap := " "
	if keybarWidth(gap) > w {
		gap = ""
	}
	r := make(srow, 0, len(fkeys)*2)
	for _, kv := range fkeys {
		r = append(r, span{kv.k, styleKeyNum}, span{gap + kv.l + " ", styleKeyLabel})
	}
	return r.pad(w, styleKeyLabel)
}

func keybarWidth(gap string) int {
	n := 0
	for _, kv := range fkeys {
		n += len(kv.k) + len(gap) + len(kv.l) + 1
	}
	return n
}

func expandTilde(p string) string {
	if p == "~" {
		h, _ := os.UserHomeDir()
		return h
	}
	if strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, p[2:])
	}
	return p
}
