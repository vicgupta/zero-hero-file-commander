package main

import (
	"github.com/charmbracelet/lipgloss"
)

// theme is the small set of semantic colors every screen element is built
// from. Adding a theme means adding one literal below — nothing else in the
// codebase names a color directly.
type theme struct {
	panelBg lipgloss.Color // panel body / command line / key-bar background
	panelFg lipgloss.Color // default text on panelBg

	dirFg lipgloss.Color // directory names
	tagFg lipgloss.Color // tagged/selected files

	// cursorBg is the one accent background used everywhere something is
	// "highlighted": the file-cursor bar, key-bar labels, dialog selection,
	// and dialog/viewer titles. cursorFg is the text color guaranteed to
	// read on it — reuse this pair together; don't pair cursorBg with boxFg,
	// which is only calibrated to read on boxBg.
	cursorBg lipgloss.Color
	cursorFg lipgloss.Color

	boxBg lipgloss.Color // active-panel header / dialog box background
	boxFg lipgloss.Color // text on boxBg

	idleFg  lipgloss.Color // inactive panel header
	mutedFg lipgloss.Color // idle-caret (vi-navigation) indicator

	brightFg lipgloss.Color // status line, error text, active-caret, key numbers
	blackBg  lipgloss.Color // key-number background
	dangerBg lipgloss.Color // error background
}

// themes are the built-in palettes, selectable via Options → Theme or
// cfg.Theme. "norton" is the default: the classic light-grey-on-DOS-blue
// scheme with a cyan cursor bar and yellow tagged files (spec §3.1, §12.4).
var themes = map[string]theme{
	"norton": {
		panelBg: "#0000AA", panelFg: "#AAAAAA",
		dirFg: "#FFFFFF", tagFg: "#FFFF55",
		cursorBg: "#00AAAA", cursorFg: "#000000",
		boxBg: "#AAAAAA", boxFg: "#000000",
		idleFg: "#55FFFF", mutedFg: "#555555",
		brightFg: "#FFFFFF", blackBg: "#000000", dangerBg: "#AA0000",
	},
	// Sarah Drasner's Night Owl: deep navy, soft blue-grey text, function-blue
	// accents. This was the project's original (pre-review) color scheme.
	"nightowl": {
		panelBg: "#011627", panelFg: "#d6deeb",
		dirFg: "#82aaff", tagFg: "#ffcb8b",
		cursorBg: "#1d3b53", cursorFg: "#d6deeb",
		boxBg: "#82aaff", boxFg: "#011627",
		idleFg: "#637777", mutedFg: "#3f5769",
		brightFg: "#ffffff", blackBg: "#011627", dangerBg: "#c0392b",
	},
	// A dark, warm palette in the spirit of the opencode CLI's terminal UI —
	// not pulled from an official spec, since none was available to check
	// against; charcoal background with an amber accent.
	"opencode": {
		panelBg: "#1a1a1a", panelFg: "#e0e0e0",
		dirFg: "#ffffff", tagFg: "#fab283",
		cursorBg: "#fab283", cursorFg: "#1a1a1a",
		boxBg: "#333333", boxFg: "#fab283",
		idleFg: "#808080", mutedFg: "#5c5c5c",
		brightFg: "#ffffff", blackBg: "#000000", dangerBg: "#c0392b",
	},
}

const defaultTheme = "norton"

// themeNames lists built-in themes in a stable, deliberate order (not map
// iteration order) for menus and validation messages.
var themeNames = []string{"norton", "nightowl", "opencode"}

var (
	styleDefault  lipgloss.Style
	styleDir      lipgloss.Style
	styleSelected lipgloss.Style

	styleCursor         lipgloss.Style
	styleCursorDir      lipgloss.Style
	styleCursorSelected lipgloss.Style

	styleActiveHeader   lipgloss.Style
	styleInactiveHeader lipgloss.Style
	styleDivider        lipgloss.Style

	styleCmdline      lipgloss.Style
	styleCmdCaret     lipgloss.Style
	styleCmdCaretIdle lipgloss.Style
	styleStatus       lipgloss.Style
	styleError        lipgloss.Style

	styleKeyNum   lipgloss.Style
	styleKeyLabel lipgloss.Style

	styleDialog    lipgloss.Style
	styleDialogSel lipgloss.Style
	styleTitle     lipgloss.Style
	styleViewer    lipgloss.Style

	// Markdown highlighting (viewer/Quick View), built from the same semantic
	// theme fields as everything else — no theme gets its own markdown colors.
	styleMdH1        lipgloss.Style
	styleMdH2        lipgloss.Style
	styleMdH3        lipgloss.Style
	styleMdBold      lipgloss.Style
	styleMdItalic    lipgloss.Style
	styleMdCode      lipgloss.Style
	styleMdCodeBlock lipgloss.Style
	styleMdQuote     lipgloss.Style
	styleMdBullet    lipgloss.Style
	styleMdRule      lipgloss.Style
	styleMdLinkText  lipgloss.Style
	styleMdLinkURL   lipgloss.Style
)

var activeTheme = defaultTheme

// applyTheme rebuilds every style var from the named theme. Unknown names are
// ignored in favor of the current theme, so a stale or hand-edited config
// value can never leave the style vars unset.
func applyTheme(name string) {
	t, ok := themes[name]
	if !ok {
		return
	}
	activeTheme = name

	styleDefault = lipgloss.NewStyle().Foreground(t.panelFg).Background(t.panelBg)
	styleDir = lipgloss.NewStyle().Foreground(t.dirFg).Background(t.panelBg).Bold(true)
	styleSelected = lipgloss.NewStyle().Foreground(t.tagFg).Background(t.panelBg).Bold(true)

	styleCursor = lipgloss.NewStyle().Foreground(t.cursorFg).Background(t.cursorBg)
	styleCursorDir = lipgloss.NewStyle().Foreground(t.cursorFg).Background(t.cursorBg).Bold(true)
	styleCursorSelected = lipgloss.NewStyle().Foreground(t.tagFg).Background(t.cursorBg).Bold(true)

	styleActiveHeader = lipgloss.NewStyle().Foreground(t.boxFg).Background(t.boxBg).Bold(true)
	styleInactiveHeader = lipgloss.NewStyle().Foreground(t.idleFg).Background(t.panelBg)
	styleDivider = lipgloss.NewStyle().Foreground(t.panelFg).Background(t.panelBg)

	styleCmdline = lipgloss.NewStyle().Foreground(t.panelFg).Background(t.panelBg)
	styleCmdCaret = lipgloss.NewStyle().Foreground(t.brightFg).Background(t.panelBg).Bold(true)
	styleCmdCaretIdle = lipgloss.NewStyle().Foreground(t.mutedFg).Background(t.panelBg)
	styleStatus = lipgloss.NewStyle().Foreground(t.brightFg).Background(t.panelBg).Bold(true)
	styleError = lipgloss.NewStyle().Foreground(t.brightFg).Background(t.dangerBg).Bold(true)

	styleKeyNum = lipgloss.NewStyle().Foreground(t.brightFg).Background(t.blackBg)
	// cursorFg, not boxFg: boxFg is calibrated to read on boxBg, and cursorBg
	// isn't guaranteed to share boxBg's lightness (nightowl and opencode's
	// cursorBg is dark, where boxFg is unreadable or — for opencode — the
	// exact same color as cursorBg). cursorFg is already proven to contrast
	// against cursorBg by the file-cursor bar itself.
	styleKeyLabel = lipgloss.NewStyle().Foreground(t.cursorFg).Background(t.cursorBg)

	styleDialog = lipgloss.NewStyle().Foreground(t.boxFg).Background(t.boxBg)
	styleDialogSel = lipgloss.NewStyle().Foreground(t.cursorFg).Background(t.cursorBg).Bold(true)
	styleTitle = lipgloss.NewStyle().Foreground(t.cursorFg).Background(t.cursorBg).Bold(true)
	styleViewer = lipgloss.NewStyle().Foreground(t.panelFg).Background(t.panelBg)

	styleMdH1 = lipgloss.NewStyle().Foreground(t.dirFg).Background(t.panelBg).Bold(true).Underline(true)
	styleMdH2 = lipgloss.NewStyle().Foreground(t.dirFg).Background(t.panelBg).Bold(true)
	styleMdH3 = lipgloss.NewStyle().Foreground(t.tagFg).Background(t.panelBg).Bold(true)
	styleMdBold = lipgloss.NewStyle().Foreground(t.panelFg).Background(t.panelBg).Bold(true)
	styleMdItalic = lipgloss.NewStyle().Foreground(t.panelFg).Background(t.panelBg).Italic(true)
	styleMdCode = lipgloss.NewStyle().Foreground(t.cursorFg).Background(t.cursorBg)
	styleMdCodeBlock = lipgloss.NewStyle().Foreground(t.cursorFg).Background(t.cursorBg)
	styleMdQuote = lipgloss.NewStyle().Foreground(t.mutedFg).Background(t.panelBg).Italic(true)
	styleMdBullet = lipgloss.NewStyle().Foreground(t.tagFg).Background(t.panelBg).Bold(true)
	styleMdRule = lipgloss.NewStyle().Foreground(t.idleFg).Background(t.panelBg)
	styleMdLinkText = lipgloss.NewStyle().Foreground(t.dirFg).Background(t.panelBg).Underline(true)
	styleMdLinkURL = lipgloss.NewStyle().Foreground(t.mutedFg).Background(t.panelBg)
}

func init() { applyTheme(defaultTheme) }
