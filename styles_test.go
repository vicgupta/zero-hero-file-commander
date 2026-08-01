package main

import "testing"

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
