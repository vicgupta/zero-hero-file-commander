package main

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// restoreTheme returns activeTheme to whatever it was before the test, since
// applyTheme mutates package-level style vars shared across the whole suite.
func restoreTheme(t *testing.T) {
	t.Helper()
	prev := activeTheme
	t.Cleanup(func() { applyTheme(prev) })
}

func TestThemeNamesMatchTheRegistry(t *testing.T) {
	if len(themeNames) != len(themes) {
		t.Fatalf("themeNames has %d entries, themes has %d — they must list the same set", len(themeNames), len(themes))
	}
	for _, n := range themeNames {
		if _, ok := themes[n]; !ok {
			t.Errorf("themeNames contains %q, which is not in the themes registry", n)
		}
		if _, ok := themeLabels[n]; !ok {
			t.Errorf("theme %q has no menu label", n)
		}
	}
}

func TestApplyThemeSwitchesActiveThemeAndStyles(t *testing.T) {
	withColor(t)
	restoreTheme(t)
	applyTheme("nightowl")
	if activeTheme != "nightowl" {
		t.Fatalf("activeTheme = %q, want nightowl", activeTheme)
	}
	night := styleDir
	applyTheme("opencode")
	if activeTheme != "opencode" {
		t.Fatalf("activeTheme = %q, want opencode", activeTheme)
	}
	if styleDir.Render("x") == night.Render("x") {
		t.Error("switching themes did not change the rendered style")
	}
}

// An unknown theme name (a stale config value, a typo) must leave the current
// theme in place rather than zeroing out the style vars.
func TestApplyThemeIgnoresUnknownNames(t *testing.T) {
	withColor(t)
	restoreTheme(t)
	applyTheme("norton")
	before := styleDefault.Render("x")
	applyTheme("does-not-exist")
	if activeTheme != "norton" {
		t.Errorf("activeTheme = %q, want norton (unchanged)", activeTheme)
	}
	if got := styleDefault.Render("x"); got != before {
		t.Errorf("an unknown theme name altered the style vars: %q -> %q", before, got)
	}
}

func TestEveryThemeProducesDistinctCursorAndDefaultStyles(t *testing.T) {
	withColor(t)
	restoreTheme(t)
	for _, name := range themeNames {
		applyTheme(name)
		if styleCursor.Render("x") == styleDefault.Render("x") {
			t.Errorf("theme %q: cursor row is not visually distinct from a plain row", name)
		}
		if styleSelected.Render("x") == styleDefault.Render("x") {
			t.Errorf("theme %q: a tagged file is not visually distinct from a plain row", name)
		}
	}
}

// relLuminance computes the WCAG relative luminance of a "#RRGGBB" color.
func relLuminance(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")
	toLin := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	rgb, _ := strconv.ParseUint(hex, 16, 32)
	r := toLin(uint8(rgb >> 16))
	g := toLin(uint8(rgb >> 8))
	b := toLin(uint8(rgb))
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// contrastRatio is the WCAG contrast ratio between two "#RRGGBB" colors.
func contrastRatio(a, b string) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestThemeContrastMeetsWCAGAA guards against exactly the bug that shipped:
// styleKeyLabel, styleDialogSel and styleTitle used to pair boxFg (only
// calibrated to read on boxBg) with cursorBg, which was unreadable in
// nightowl (1.57:1) and literally invisible in opencode (1.00:1, identical
// colors). Every foreground/background pair actually used together by a
// style must clear WCAG AA's 4.5:1 minimum for normal text, in every theme —
// present and future.
func TestThemeContrastMeetsWCAGAA(t *testing.T) {
	const minRatio = 4.5
	pairs := []struct{ name, fgField, bgField string }{
		{"default text", "panelFg", "panelBg"},
		{"directory name", "dirFg", "panelBg"},
		{"tagged file", "tagFg", "panelBg"},
		{"cursor row / key label / dialog selection / title", "cursorFg", "cursorBg"},
		{"active header / dialog box", "boxFg", "boxBg"},
		{"status line / key numbers", "brightFg", "panelBg"},
		{"error line", "brightFg", "dangerBg"},
		{"key numbers", "brightFg", "blackBg"},
	}
	field := func(th theme, name string) string {
		switch name {
		case "panelFg":
			return string(th.panelFg)
		case "panelBg":
			return string(th.panelBg)
		case "dirFg":
			return string(th.dirFg)
		case "tagFg":
			return string(th.tagFg)
		case "cursorFg":
			return string(th.cursorFg)
		case "cursorBg":
			return string(th.cursorBg)
		case "boxFg":
			return string(th.boxFg)
		case "boxBg":
			return string(th.boxBg)
		case "brightFg":
			return string(th.brightFg)
		case "dangerBg":
			return string(th.dangerBg)
		case "blackBg":
			return string(th.blackBg)
		}
		t.Fatalf("unknown field %q", name)
		return ""
	}
	for _, themeName := range themeNames {
		th := themes[themeName]
		for _, p := range pairs {
			fg, bg := field(th, p.fgField), field(th, p.bgField)
			ratio := contrastRatio(fg, bg)
			if ratio < minRatio {
				t.Errorf("theme %q, %s: contrast %.2f:1 (%s on %s), want >= %.1f:1",
					themeName, p.name, ratio, fg, bg, minRatio)
			}
		}
	}
}
