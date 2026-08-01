package main

import "testing"

func TestResolveDirsDefaultsToCurrentDirectory(t *testing.T) {
	l, r := resolveDirs("", "", nil, "/home/vic/project")
	if l != "/home/vic/project" || r != "/home/vic/project" {
		t.Errorf("got (%q, %q), want both panels at the cwd", l, r)
	}
}

func TestResolveDirsPositionalArgsWin(t *testing.T) {
	l, r := resolveDirs("", "", []string{"/one"}, "/cwd")
	if l != "/one" || r != "/one" {
		t.Errorf("zhfc /one -> got (%q, %q), want (/one, /one)", l, r)
	}
	l, r = resolveDirs("", "", []string{"/one", "/two"}, "/cwd")
	if l != "/one" || r != "/two" {
		t.Errorf("zhfc /one /two -> got (%q, %q), want (/one, /two)", l, r)
	}
}

func TestResolveDirsFlagsWin(t *testing.T) {
	l, r := resolveDirs("/flagL", "/flagR", nil, "/cwd")
	if l != "/flagL" || r != "/flagR" {
		t.Errorf("got (%q, %q), want (/flagL, /flagR)", l, r)
	}
}

// A directory named for one side only must not silently pull in the cwd on
// the other side when it should instead mirror the side that was given.
func TestResolveDirsOneSidedFlagMirrorsIntoTheOther(t *testing.T) {
	l, r := resolveDirs("/only-left", "", nil, "/cwd")
	if l != "/only-left" || r != "/only-left" {
		t.Errorf("got (%q, %q), want (/only-left, /only-left)", l, r)
	}
}

func TestParseFlagsDefaultsToZeroValues(t *testing.T) {
	_, f, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags(nil): %v", err)
	}
	if f.left != "" || f.right != "" || f.theme != "" || f.hidden || f.brief || f.viKeys || f.version {
		t.Errorf("unexpected non-zero defaults: %+v", f)
	}
}

func TestParseFlagsReadsEveryOption(t *testing.T) {
	fs, f, err := parseFlags([]string{
		"-left", "/l", "-right", "/r", "-theme", "nightowl",
		"-hidden", "-brief", "-vi-keys", "/pos1", "/pos2",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if f.left != "/l" || f.right != "/r" || f.theme != "nightowl" {
		t.Errorf("string flags = %+v", f)
	}
	if !f.hidden || !f.brief || !f.viKeys {
		t.Errorf("bool flags = %+v", f)
	}
	if got := fs.Args(); len(got) != 2 || got[0] != "/pos1" || got[1] != "/pos2" {
		t.Errorf("positional args = %v, want [/pos1 /pos2]", got)
	}
}

func TestParseFlagsVersionFlag(t *testing.T) {
	_, f, err := parseFlags([]string{"-version"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if !f.version {
		t.Error("-version should set f.version")
	}
}

func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	if _, _, err := parseFlags([]string{"-does-not-exist"}); err == nil {
		t.Error("an unknown flag should return an error, not exit the process")
	}
}

// The bug this guards against: a bool flag with its Go zero value (false)
// must not be indistinguishable from "not passed" — otherwise starting zhfc
// with no flags at all would silently reset every saved true back to false.
func TestApplyFlagsOnlyOverridesFlagsThatWerePassed(t *testing.T) {
	saved := cfg{Hidden: true, Brief: true, ViKeys: true, Theme: "opencode"}
	fs, f, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyFlags(saved, fs, f)
	if err != nil {
		t.Fatal(err)
	}
	if got != saved {
		t.Errorf("no flags passed: config changed from %+v to %+v", saved, got)
	}
}

func TestApplyFlagsOverridesExplicitlyPassedFalse(t *testing.T) {
	saved := cfg{Hidden: true, Brief: true, ViKeys: true}
	fs, f, err := parseFlags([]string{"-hidden=false", "-brief=false", "-vi-keys=false"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyFlags(saved, fs, f)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hidden || got.Brief || got.ViKeys {
		t.Errorf("explicit =false flags did not override the saved config: %+v", got)
	}
}

func TestApplyFlagsAcceptsAKnownTheme(t *testing.T) {
	fs, f, err := parseFlags([]string{"-theme", "nightowl"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyFlags(cfg{Theme: "norton"}, fs, f)
	if err != nil {
		t.Fatalf("applyFlags: %v", err)
	}
	if got.Theme != "nightowl" {
		t.Errorf("theme = %q, want nightowl", got.Theme)
	}
}

func TestApplyFlagsRejectsAnUnknownTheme(t *testing.T) {
	fs, f, err := parseFlags([]string{"-theme", "bogus"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyFlags(cfg{Theme: "norton"}, fs, f)
	if err == nil {
		t.Fatal("an unknown theme name should produce an error")
	}
	if got.Theme != "norton" {
		t.Errorf("theme should be left unchanged on error, got %q", got.Theme)
	}
}
