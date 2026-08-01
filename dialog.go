package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type dlgKind int

const (
	dlgMenu dlgKind = iota
	dlgInput
	dlgConfirm
	dlgHelp
	dlgViewer
	dlgConflict
)

// menuItem is one entry in a menu dialog.
type menuItem struct {
	label  string
	action string
}

// dialog is the generic overlay: menu, input, confirm, help, or viewer.
type dialog struct {
	kind   dlgKind
	title  string
	prompt string
	items  []menuItem
	sel    int // menu selection or scroll offset (help/viewer)
	value  []rune
	vcur   int
	action string // action dispatched on submit
	viewer *viewer
}

func newMenuDialog(title string, items []menuItem) *dialog {
	return &dialog{kind: dlgMenu, title: title, items: items}
}

func newInputDialog(title, prompt, initial, action string) *dialog {
	return &dialog{kind: dlgInput, title: title, prompt: prompt, value: []rune(initial), vcur: len(initial), action: action}
}

func newConfirmDialog(msg, action string) *dialog {
	return &dialog{kind: dlgConfirm, title: "Confirm", prompt: msg, action: action}
}

func newHelpDialog() *dialog {
	return &dialog{kind: dlgHelp, title: " Help "}
}

func newViewerDialog(v *viewer) *dialog {
	title := " View: " + v.path
	if v.hex {
		title += "  [hex]"
	}
	return &dialog{kind: dlgViewer, title: title, viewer: v}
}

// conflictChoices are the answers offered when a destination already exists.
// Rename (spec §9.1.2) is not offered yet.
var conflictChoices = []struct {
	label  string
	answer conflictAnswer
}{
	{"Overwrite", conflictAnswer{action: ansOverwrite}},
	{"Overwrite all", conflictAnswer{action: ansOverwrite, all: true}},
	{"Skip", conflictAnswer{action: ansSkip}},
	{"Skip all", conflictAnswer{action: ansSkip, all: true}},
	{"Cancel operation", conflictAnswer{action: ansCancel}},
}

func newConflictDialog(c *conflict) *dialog {
	items := make([]menuItem, len(conflictChoices))
	for i, ch := range conflictChoices {
		items[i] = menuItem{label: ch.label}
	}
	what := "File"
	if c.dstDir {
		what = "Directory"
	}
	prompt := strings.Join([]string{
		what + " already exists:",
		"  " + c.dst,
		"",
		"  existing:  " + padLeft(humanSize(c.dstSize), 8) + "  " + modStr(c.dstMod),
		"  new:       " + padLeft(humanSize(c.srcSize), 8) + "  " + modStr(c.srcMod),
	}, "\n")
	// Skip is the safe default, so a stray Enter never destroys data.
	return &dialog{kind: dlgConflict, title: " Overwrite? ", prompt: prompt, items: items, sel: 2}
}

func modStr(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(dateLayout)
}

// body returns the dialog's content lines (unstyled, unpadded) and the index of
// the line to highlight, or -1 for none.
func (d *dialog) body() ([]string, int) {
	switch d.kind {
	case dlgHelp:
		return helpLines, -1
	case dlgViewer:
		return d.viewer.lines, -1
	case dlgMenu, dlgConflict:
		out := []string{d.title, ""}
		if d.prompt != "" {
			out = append(out, strings.Split(d.prompt, "\n")...)
			out = append(out, "")
		}
		hl := len(out) + d.sel
		for i, it := range d.items {
			mark := "  "
			if i == d.sel {
				mark = "▶ "
			}
			out = append(out, mark+it.label)
		}
		return out, hl
	case dlgInput:
		text := string(d.value)
		r := []rune(text)
		cur := d.vcur
		if cur > len(r) {
			cur = len(r)
		}
		line := append(append([]rune{}, r[:cur]...), '▌')
		line = append(line, r[cur:]...)
		return []string{d.title, "", "> " + string(line), "", "Enter = OK    Esc = Cancel"}, -1
	case dlgConfirm:
		return []string{d.title, "", d.prompt, "", "Enter = Yes    Esc = No"}, -1
	}
	return nil, -1
}

// handleDlgKey routes keys while a dialog is open.
func (m model) handleDlgKey(msg tea.KeyMsg) (model, tea.Cmd) {
	d := m.dlg
	switch d.kind {
	case dlgViewer, dlgHelp:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyF3:
			m.dlg = nil
		case tea.KeyUp:
			d.sel--
		case tea.KeyDown:
			d.sel++
		case tea.KeyPgUp:
			d.sel -= m.height - 2
		case tea.KeyPgDown:
			d.sel += m.height - 2
		case tea.KeyHome:
			d.sel = 0
		case tea.KeyEnd:
			d.sel = 1 << 30
		}
		for _, r := range msg.Runes {
			switch r {
			case 'q', 'Q':
				m.dlg = nil
			case 'j':
				d.sel++
			case 'k':
				d.sel--
			case 'n':
				d.sel += m.height - 2
			case 'u':
				d.sel -= m.height - 2
			case 'h', 'H':
				if d.viewer != nil {
					d.viewer.toggleHex()
					d.title = " View: " + d.viewer.path
					if d.viewer.hex {
						d.title += "  [hex]"
					}
				}
			}
		}
		if d.kind == dlgViewer {
			d.sel = clamp(d.sel, 0, len(d.viewer.lines)-1)
		} else {
			d.sel = clamp(d.sel, 0, len(helpLines)-1)
		}
	case dlgConflict:
		switch msg.Type {
		case tea.KeyUp:
			d.sel--
		case tea.KeyDown:
			d.sel++
		case tea.KeyHome:
			d.sel = 0
		case tea.KeyEnd:
			d.sel = len(conflictChoices) - 1
		case tea.KeyEsc:
			return m.answerConflict(conflictAnswer{action: ansCancel})
		case tea.KeyEnter:
			if d.sel >= 0 && d.sel < len(conflictChoices) {
				return m.answerConflict(conflictChoices[d.sel].answer)
			}
		}
		d.sel = clamp(d.sel, 0, len(conflictChoices)-1)
	case dlgMenu:
		switch msg.Type {
		case tea.KeyEsc:
			m.dlg = nil
		case tea.KeyUp:
			d.sel--
		case tea.KeyDown:
			d.sel++
		case tea.KeyHome:
			d.sel = 0
		case tea.KeyEnd:
			d.sel = len(d.items) - 1
		case tea.KeyEnter:
			if d.sel >= 0 && d.sel < len(d.items) {
				action := d.items[d.sel].action
				m.dlg = nil
				if needsInput(action) {
					m.dlg = inputFor(action)
					return m, nil
				}
				return m.doAction(action, "")
			}
		}
		d.sel = clamp(d.sel, 0, len(d.items)-1)
	case dlgInput:
		switch msg.Type {
		case tea.KeyEsc:
			m.dlg = nil
		case tea.KeyEnter:
			action, val := d.action, string(d.value)
			m.dlg = nil
			return m.doAction(action, val)
		case tea.KeyBackspace, tea.KeyCtrlH:
			d.delLeft()
		case tea.KeyLeft:
			if d.vcur > 0 {
				d.vcur--
			}
		case tea.KeyRight:
			if d.vcur < len(d.value) {
				d.vcur++
			}
		case tea.KeyHome:
			d.vcur = 0
		case tea.KeyEnd:
			d.vcur = len(d.value)
		case tea.KeyCtrlU:
			d.value = d.value[:0]
			d.vcur = 0
		case tea.KeyCtrlW:
			i := d.vcur
			for i > 0 && d.value[i-1] == ' ' {
				i--
			}
			for i > 0 && d.value[i-1] != ' ' {
				i--
			}
			d.value = append(d.value[:i], d.value[d.vcur:]...)
			d.vcur = i
		default:
			if len(msg.Runes) > 0 {
				d.insert(msg.Runes)
			}
		}
	case dlgConfirm:
		switch msg.Type {
		case tea.KeyEnter:
			action := d.action
			m.dlg = nil
			return m.doAction(action, "")
		case tea.KeyEsc:
			m.dlg = nil
		}
	}
	return m, nil
}

func needsInput(action string) bool {
	return action == "select_mask" || action == "unselect_mask" || action == "newfile"
}

func inputFor(action string) *dialog {
	switch action {
	case "select_mask":
		return newInputDialog("Select files", "Mask (e.g. *.go;*.md):", "*", action)
	case "unselect_mask":
		return newInputDialog("Unselect files", "Mask (e.g. *.tmp):", "", action)
	case "newfile":
		return newInputDialog("New file", "File name:", "", action)
	}
	return nil
}

func (d *dialog) insert(r []rune) {
	head := append([]rune{}, d.value[:d.vcur]...)
	tail := d.value[d.vcur:]
	d.value = append(head, r...)
	d.value = append(d.value, tail...)
	d.vcur += len(r)
}

func (d *dialog) delLeft() {
	if d.vcur > 0 {
		d.value = append(d.value[:d.vcur-1], d.value[d.vcur:]...)
		d.vcur--
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// dialogRows overlays the active dialog onto the base screen.
func (m model) dialogRows(base []srow) []srow {
	if k := m.dlg.kind; k == dlgHelp || k == dlgViewer {
		return m.fullScreenRows(len(base))
	}
	return m.boxedRows(base)
}

// fullScreenRows draws the help/viewer overlay, which replaces the screen.
func (m model) fullScreenRows(h int) []srow {
	d := m.dlg
	lines, _ := d.body()
	out := make([]srow, 0, h)
	out = append(out, srow{{padRune(" "+d.title, m.width), styleTitle}})
	for i := 1; i < h; i++ {
		var text string
		if n := d.sel + i - 1; n >= 0 && n < len(lines) {
			text = lines[n]
		}
		out = append(out, srow{{padRune(" "+text, m.width), styleViewer}})
	}
	return out
}

// boxedRows centres a bordered dialog over the panels.
func (m model) boxedRows(base []srow) []srow {
	d := m.dlg
	body, hl := d.body()
	bw := 0
	for _, l := range body {
		if n := len([]rune(l)); n > bw {
			bw = n
		}
	}
	bw += 4 // border + padding
	if bw > m.width-2 {
		bw = m.width - 2
	}
	if bw < 8 {
		bw = 8
	}

	box := make([]srow, 0, len(body)+2)
	box = append(box, srow{{"┌" + strings.Repeat("─", bw-2) + "┐", styleDialog}})
	for i, l := range body {
		st := styleDialog
		if i == hl {
			st = styleDialogSel
		}
		box = append(box, srow{
			{"│ ", styleDialog},
			{padRune(l, bw-4), st},
			{" │", styleDialog},
		})
	}
	box = append(box, srow{{"└" + strings.Repeat("─", bw-2) + "┘", styleDialog}})

	x := (m.width - bw) / 2
	if x < 0 {
		x = 0
	}
	y := (len(base) - len(box)) / 2
	if y < 0 {
		y = 0
	}
	for i, bl := range box {
		if y+i >= len(base) {
			break
		}
		base[y+i] = base[y+i].overlay(x, bl)
	}
	return base
}

var helpLines = []string{
	" Zero Hero File Commander (zhfc) — quick help",
	"",
	" F1 Help     F2 User menu    F3 View      F4 Edit",
	" F5 Copy     F6 Move         F7 MkDir     F8 Delete",
	" F9 Commands F10 Quit        F11 chmod    Ctrl+Q Quit",
	"",
	" Navigation:",
	"   arrows / PgUp PgDn / Home End   move cursor",
	"   j / k                             down / up",
	"   h / l                             up one level / enter directory",
	"   n / u                             page down / page up",
	"   Right or Enter on a directory     enter it",
	"   Left (empty command line)         up one level",
	"   Enter on a file                   open the viewer",
	"   Tab                               switch active panel",
	"",
	" Fast search:",
	"   with the command line empty, type letters to jump to a matching",
	"   filename (e.g. 'sb' jumps to sbin). A letter that matches nothing",
	"   is typed into the command line instead, so ordinary commands still",
	"   work without needing ':' first in the common case.",
	"",
	" Command line (bottom row):",
	"   type a command, then Enter to run it (ls -la, cd /tmp, ...)",
	"   h j k l n u are reserved for navigation while the command line",
	"   is empty; press ':' first to type a command starting with one of",
	"   them (e.g. :ls -la).",
	"   A bright caret means keys are typed, a dim one that they navigate.",
	"   cd <dir> changes the active panel   '..' goes up",
	"   Ctrl+E recall last command",
	"   Esc clears the command line and any in-progress fast search",
	"",
	" Selection:",
	"   Insert / Space toggle   Ctrl+A select all",
	"   Ctrl+T invert           Ctrl+D clear",
	"   F5/F6/F8 act on the selection if any, else the cursor row",
	"",
	" Other:",
	"   Ctrl+R reread    Ctrl+U swap panels    Ctrl+B brief/full",
	"   F11 change permissions (octal mode)",
	"   F9 Commands -> Theme...  switch color scheme (norton/nightowl/opencode)",
	"",
	" Press q or Esc to close.",
}
