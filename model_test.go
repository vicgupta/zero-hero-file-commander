package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// testModel returns a model whose panels point at a fresh temp dir with one file.
func testModel(t *testing.T) model {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "one.txt"))
	m := newModel(dir, dir, cfg{})
	m.panels[0].setCursor(1) // first real file, not ".."
	m.width, m.height = 100, 30
	return m
}

func TestViewRendersPanelsAndKeybar(t *testing.T) {
	m := testModel(t)
	v := m.View()
	if len(v) == 0 {
		t.Fatal("View returned empty")
	}
	if !strings.Contains(v, "F10 Quit") {
		t.Error("keybar should contain F10 Quit")
	}
	if !strings.Contains(v, "│") {
		t.Error("View should contain a panel divider")
	}
}

func TestF10Quits(t *testing.T) {
	m := testModel(t)
	nm, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyF10})
	if cmd == nil {
		t.Fatal("F10 should return a quit command")
	}
	if len(nm.cmdline.text) != 0 {
		t.Error("unexpected")
	}
}

func TestCtrlLJumpsToRightPanelWithCommandLineFocused(t *testing.T) {
	m := testModel(t)
	if m.active != 0 {
		t.Fatalf("setup: expected left panel active, got %d", m.active)
	}
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	if m.active != 1 {
		t.Errorf("active panel = %d, want 1 (right)", m.active)
	}
	if !m.cmdFocus {
		t.Error("Ctrl+L should focus the command line")
	}
}

func TestCapitalSCyclesSortAndHiddenFiles(t *testing.T) {
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "one.txt"))
	mustWrite(t, filepath.Join(dir, ".hidden"))
	m := newModel(dir, dir, cfg{}) // cfg{}.Hidden == false: dotfiles off by default
	m.width, m.height = 100, 30
	p := m.panels[m.active]

	hasHidden := func() bool {
		for _, e := range p.entries {
			if e.name == ".hidden" {
				return true
			}
		}
		return false
	}

	if p.sortMode != "name" || hasHidden() {
		t.Fatalf("setup: expected name sort with dotfiles hidden, got sortMode=%q hasHidden=%v", p.sortMode, hasHidden())
	}

	m, _ = m.handleKey(key('S')) // -> size
	if p.sortMode != "size" {
		t.Errorf("step 1: sortMode = %q, want size", p.sortMode)
	}
	m, _ = m.handleKey(key('S')) // -> time
	if p.sortMode != "time" {
		t.Errorf("step 2: sortMode = %q, want time", p.sortMode)
	}
	m, _ = m.handleKey(key('S')) // -> hide dotfiles
	if hasHidden() {
		t.Error("step 3: dotfiles should be hidden")
	}
	m, _ = m.handleKey(key('S')) // -> show dotfiles
	if !hasHidden() {
		t.Error("step 4: dotfiles should be shown")
	}
	m, _ = m.handleKey(key('S')) // -> back to name, dotfiles restored to config default (off)
	if p.sortMode != "name" {
		t.Errorf("step 5 (wrap): sortMode = %q, want name", p.sortMode)
	}
	if hasHidden() {
		t.Error("step 5 (wrap): dotfiles should be hidden again (restored to config default)")
	}
}

func TestF7OpensMkdirDialog(t *testing.T) {
	m := testModel(t)
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyF7})
	if m.dlg == nil || m.dlg.kind != dlgInput || m.dlg.action != "mkdir" {
		t.Fatalf("F7 should open mkdir input dialog, got %+v", m.dlg)
	}
}

func TestPrintableKeysGoToCommandLine(t *testing.T) {
	m := testModel(t)
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ls -la")})
	if string(m.cmdline.text) != "ls -la" {
		t.Fatalf("cmdline = %q, want %q", string(m.cmdline.text), "ls -la")
	}
	// Esc clears
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.cmdline.text) != 0 {
		t.Fatalf("cmdline not cleared: %q", string(m.cmdline.text))
	}
}

func TestCopyFlowOpensDialog(t *testing.T) {
	dir := t.TempDir()
	_ = dir
	m := testModel(t)
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyF5})
	if m.dlg == nil || m.dlg.action != "copy" {
		t.Fatalf("F5 should open copy dialog, got %+v", m.dlg)
	}
}

func TestSwapPanels(t *testing.T) {
	m := testModel(t)
	a, b := m.panels[0], m.panels[1]
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.panels[0] != b || m.panels[1] != a {
		t.Error("Ctrl+U should swap panels")
	}
}

// pump drives the model the way Bubble Tea's event loop does: run the pending
// command, feed its message back into Update, repeat. When a collision dialog
// appears it picks the choice named by want. It runs under a deadline so a
// deadlock in the operation handshake fails the test instead of hanging it.
func pump(t *testing.T, m model, cmd tea.Cmd, choice string) model {
	t.Helper()
	type result struct{ m model }
	done := make(chan result, 1)
	go func() {
		answered := false
		for i := 0; cmd != nil && i < 100; i++ {
			msg := cmd()
			tm, next := m.Update(msg)
			m, cmd = tm.(model), next
			if m.dlg != nil && m.dlg.kind == dlgConflict && !answered {
				answered = true
				for j, c := range conflictChoices {
					if c.label == choice {
						m.dlg.sel = j
					}
				}
				m, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			}
		}
		done <- result{m}
	}()
	select {
	case r := <-done:
		return r.m
	case <-time.After(10 * time.Second):
		t.Fatal("operation deadlocked waiting on the conflict handshake")
		return m
	}
}

func conflictModel(t *testing.T) (model, string, string) {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	srcDir, dstDir := t.TempDir(), t.TempDir()
	write(t, filepath.Join(srcDir, "f.txt"), "new")
	dst := write(t, filepath.Join(dstDir, "f.txt"), "old")
	m := newModel(srcDir, dstDir, cfg{})
	m.width, m.height = 100, 30
	return m, srcDir, dst
}

func TestCopyPromptsBeforeOverwritingAndAppliesTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		choice string
		want   string
	}{
		{"Overwrite", "new"},
		{"Skip", "old"},
		{"Cancel operation", "old"},
	} {
		t.Run(tc.choice, func(t *testing.T) {
			m, srcDir, dst := conflictModel(t)
			m, cmd := m.startCopy(srcDir, filepath.Dir(dst), []entry{{name: "f.txt"}})
			m = pump(t, m, cmd, tc.choice)

			if m.opRun {
				t.Error("operation still marked as running")
			}
			if m.dlg != nil {
				t.Errorf("dialog left open: %+v", m.dlg)
			}
			if got := read(t, dst); got != tc.want {
				t.Errorf("destination = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCopyToMissingDestinationIsRejectedUpFront(t *testing.T) {
	m, srcDir, _ := conflictModel(t)
	m, cmd := m.startCopy(srcDir, filepath.Join(srcDir, "nope"), []entry{{name: "f.txt"}})
	if cmd != nil {
		t.Error("no operation should start for a missing destination")
	}
	if !m.statusErr || !strings.Contains(m.status, "does not exist") {
		t.Errorf("status = %q (err=%v), want a 'does not exist' error", m.status, m.statusErr)
	}
}

func TestKeysDuringAnOperationDoNotSpawnExtraReceivers(t *testing.T) {
	m := testModel(t)
	m.opRun = true
	for _, k := range []tea.KeyMsg{{Type: tea.KeyF5}, {Type: tea.KeyDown}, {Type: tea.KeyRunes, Runes: []rune("x")}} {
		if _, cmd := m.handleKey(k); cmd != nil {
			t.Errorf("key %v during an operation returned a command; exactly one opSub must be outstanding", k)
		}
	}
	// A window resize must not add one either.
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 25}); cmd != nil {
		t.Error("resize during an operation returned a command")
	}
}

func TestViewerHexDump(t *testing.T) {
	lines := hexDump([]byte{0x48, 0x65, 0x6c, 0x6c, 0x6f, 0, 1, 2, 0xff})
	if len(lines) != 1 {
		t.Fatalf("hexDump lines = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "48 65 6c 6c 6f") {
		t.Errorf("hexDump missing bytes: %q", lines[0])
	}
	if !strings.Contains(lines[0], "Hello...") {
		t.Errorf("hexDump missing ascii column: %q", lines[0])
	}
}

func TestTextLinesSanitizesControlChars(t *testing.T) {
	lines := textLines([]byte("a\x1b[31mb\nc\x00d"))
	if !strings.Contains(lines[0], "?") {
		t.Errorf("control chars should be replaced: %q", lines[0])
	}
	if strings.Contains(lines[0], "\x1b") {
		t.Errorf("escape sequence leaked into output: %q", lines[0])
	}
}

func viModel(t *testing.T) model {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"} {
		mustWrite(t, filepath.Join(dir, n))
	}
	m := newModel(dir, dir, cfg{ViKeys: true})
	m.width, m.height = 100, 10 // panelRows() == 7
	return m
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// viModelWithSubdir is like viModel but includes a subdirectory, for exercising
// h/l/v against a directory as well as a plain file.
func viModelWithSubdir(t *testing.T) model {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"))
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(sub, "inner.txt"))
	m := newModel(dir, dir, cfg{ViKeys: true})
	m.width, m.height = 100, 10
	return m
}

func TestHAndLNavigateLikeLeftAndRightArrows(t *testing.T) {
	m := viModelWithSubdir(t)
	p := m.panels[m.active]
	for i, e := range p.entries {
		if e.name == "sub" {
			p.setCursor(i)
		}
	}
	start := p.path

	m, _ = m.handleKey(key('l'))
	if p.path != filepath.Join(start, "sub") {
		t.Fatalf("l: path = %q, want entering sub", p.path)
	}
	if m.dlg != nil {
		t.Error("l on a directory should not open a dialog")
	}

	m, _ = m.handleKey(key('h'))
	if p.path != start {
		t.Errorf("h: path = %q, want %q", p.path, start)
	}
	if len(m.cmdline.text) != 0 {
		t.Errorf("h/l leaked into the command line: %q", string(m.cmdline.text))
	}
}

// 'v' used to be a reserved "view file" shortcut; it's been removed, so it
// must behave like any other ordinary letter now: fast search if something
// matches, otherwise typed into the command line.
func TestVIsNoLongerAReservedViewShortcut(t *testing.T) {
	m := viModelWithSubdir(t)
	p := m.panels[m.active]
	for i, e := range p.entries {
		if e.name == "a.txt" {
			p.setCursor(i)
		}
	}
	start := p.path
	m, _ = m.handleKey(key('v'))
	if m.dlg != nil {
		t.Fatalf("v should no longer open the viewer, got %+v", m.dlg)
	}
	if p.path != start {
		t.Errorf("v should not change the panel path, got %q", p.path)
	}
	// No entry in the fixture starts with "v", so it falls through to the
	// command line rather than being consumed by a fast search match.
	if got := string(m.cmdline.text); got != "v" {
		t.Errorf("command line = %q, want %q", got, "v")
	}
}

func TestViKeysNavigateWhileTheCommandLineIsDormant(t *testing.T) {
	m := viModel(t)
	p := m.panels[m.active]
	p.setCursor(0)

	m, _ = m.handleKey(key('j'))
	if p.cursor != 1 {
		t.Errorf("j: cursor = %d, want 1", p.cursor)
	}
	m, _ = m.handleKey(key('j'))
	m, _ = m.handleKey(key('k'))
	if p.cursor != 1 {
		t.Errorf("j j k: cursor = %d, want 1", p.cursor)
	}
	m, _ = m.handleKey(key('n'))
	if p.cursor != len(p.entries)-1 {
		t.Errorf("n: cursor = %d, want last (%d)", p.cursor, len(p.entries)-1)
	}
	m, _ = m.handleKey(key('u'))
	if p.cursor != 0 {
		t.Errorf("u: cursor = %d, want 0", p.cursor)
	}
	if len(m.cmdline.text) != 0 {
		t.Errorf("navigation keys leaked into the command line: %q", string(m.cmdline.text))
	}
}

// The reason ':' exists: "du -sh" must still be typeable.
func TestColonFocusesCommandLineSoDuIsTypeable(t *testing.T) {
	m := viModel(t)
	start := m.panels[m.active].cursor

	m, _ = m.handleKey(key(':'))
	if !m.cmdFocus {
		t.Fatal("':' should focus the command line")
	}
	for _, r := range "du -sh" {
		m, _ = m.handleKey(key(r))
	}
	if got := string(m.cmdline.text); got != "du -sh" {
		t.Errorf("command line = %q, want %q", got, "du -sh")
	}
	if m.panels[m.active].cursor != start {
		t.Error("cursor moved while typing a command")
	}

	// Esc releases the command line back to navigation.
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.cmdFocus || len(m.cmdline.text) != 0 {
		t.Error("Esc should clear and unfocus the command line")
	}
	m, _ = m.handleKey(key('j'))
	if m.panels[m.active].cursor == start {
		t.Error("j should navigate again after Esc")
	}
}

func TestOnceTypingStartedViKeysAreInput(t *testing.T) {
	m := viModel(t)
	start := m.panels[m.active].cursor
	// "zsh" matches no file in the fixture (a.txt..e.txt), so fast search
	// never intercepts it and the cursor never moves.
	for _, r := range "zsh " {
		m, _ = m.handleKey(key(r))
	}
	for _, r := range "judknhlv" { // now these are just characters, nav keys included
		m, _ = m.handleKey(key(r))
	}
	if got := string(m.cmdline.text); got != "zsh judknhlv" {
		t.Errorf("command line = %q, want %q", got, "zsh judknhlv")
	}
	if m.panels[m.active].cursor != start {
		t.Error("cursor moved while typing")
	}
}

// fastSearchModel gives a directory with an unambiguous "s"-prefixed entry, so
// the classic fast-search example (typing "sb" jumps to "sbin") is directly
// testable.
func fastSearchModel(t *testing.T) (model, *panel, int) {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sbin"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "readme.txt"))
	m := newModel(dir, dir, cfg{ViKeys: true})
	m.width, m.height = 100, 10
	p := m.panels[m.active]
	sbin := -1
	for i, e := range p.entries {
		if e.name == "sbin" {
			sbin = i
		}
	}
	if sbin < 0 {
		t.Fatal("fixture missing sbin")
	}
	return m, p, sbin
}

func TestFastSearchJumpsToMatchingFilename(t *testing.T) {
	m, p, sbin := fastSearchModel(t)
	m, _ = m.handleKey(key('s'))
	if p.cursor != sbin {
		t.Fatalf("after 's': cursor = %d, want sbin (%d)", p.cursor, sbin)
	}
	m, _ = m.handleKey(key('b'))
	if p.cursor != sbin {
		t.Fatalf("after 'sb': cursor = %d, want sbin (%d)", p.cursor, sbin)
	}
	if p.search != "sb" {
		t.Errorf("search buffer = %q, want %q", p.search, "sb")
	}
	if len(m.cmdline.text) != 0 {
		t.Errorf("a matching search should not touch the command line: %q", string(m.cmdline.text))
	}
}

// Once the typed prefix stops matching anything, fast search assumes this was
// always an ordinary command and hands the whole thing to the command line —
// so "ls" still works without requiring ':' just because some file starts
// with 'l'.
func TestFastSearchFallsBackToCommandLineOnMismatch(t *testing.T) {
	m, p, sbin := fastSearchModel(t)
	m, _ = m.handleKey(key('s')) // matches sbin
	m, _ = m.handleKey(key('z')) // "sz" matches nothing

	if got := string(m.cmdline.text); got != "sz" {
		t.Errorf("command line = %q, want %q", got, "sz")
	}
	if p.search != "" {
		t.Errorf("search buffer should be cleared after a fallback, got %q", p.search)
	}
	if p.cursor != sbin {
		t.Errorf("cursor should stay at the last successful match (%d), got %d", sbin, p.cursor)
	}

	// Typing continues normally afterward.
	m, _ = m.handleKey(key('x'))
	if got := string(m.cmdline.text); got != "szx" {
		t.Errorf("command line = %q, want %q", got, "szx")
	}
}

func TestFastSearchIsAbandonedByExplicitNavigation(t *testing.T) {
	m, p, _ := fastSearchModel(t)
	m, _ = m.handleKey(key('s'))
	if p.search == "" {
		t.Fatal("setup: expected a search in progress")
	}
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.search != "" {
		t.Error("an explicit arrow-key move should abandon the fast search")
	}
}

func TestEscClearsAnInProgressFastSearch(t *testing.T) {
	m, p, _ := fastSearchModel(t)
	m, _ = m.handleKey(key('s'))
	if p.search == "" {
		t.Fatal("setup: expected a search in progress")
	}
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if p.search != "" {
		t.Error("Esc should clear the in-progress fast search")
	}
}

func TestBackspaceNoLongerGoesUpADirectory(t *testing.T) {
	m := testModel(t)
	p := m.panels[m.active]
	start := p.path
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if p.path != start {
		t.Errorf("Backspace with an empty command line changed the path: %q -> %q", start, p.path)
	}
}

func TestBackspaceStillEditsTheCommandLine(t *testing.T) {
	m := viModel(t)
	m, _ = m.handleKey(key(':'))
	for _, r := range "abc" {
		m, _ = m.handleKey(key(r))
	}
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := string(m.cmdline.text); got != "ab" {
		t.Errorf("command line = %q, want %q", got, "ab")
	}
}

func TestViKeysCanBeDisabled(t *testing.T) {
	m := viModel(t)
	m.cfg.ViKeys = false
	start := m.panels[m.active].cursor
	m, _ = m.handleKey(key('j'))
	if m.panels[m.active].cursor != start {
		t.Error("j navigated even though vi keys are disabled")
	}
	if string(m.cmdline.text) != "j" {
		t.Errorf("command line = %q, want %q", string(m.cmdline.text), "j")
	}
}

func TestCaretShowsWhetherKeysTypeOrNavigate(t *testing.T) {
	withColor(t)
	m := viModel(t)
	w := len([]rune(m.panels[m.active].path)) + 20 // wide enough for the caret
	idle := m.cmdlineRow(w).String()
	m, _ = m.handleKey(key(':'))
	active := m.cmdlineRow(w).String()
	if idle == active {
		t.Error("caret should look different when the command line is focused")
	}
}

func TestCtrlQQuits(t *testing.T) {
	m := testModel(t)
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("Ctrl+Q should return a quit command")
	}
}

func TestCtrlQCancelsARunningOperationBeforeQuitting(t *testing.T) {
	m := testModel(t)
	cancelled := false
	m.opRun = true
	m.opCancel = func() { cancelled = true }
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("Ctrl+Q should quit even mid-operation")
	}
	if !cancelled {
		t.Error("Ctrl+Q should cancel the running operation")
	}
}

func TestThemeMenuSwitchesAndPersists(t *testing.T) {
	restoreTheme(t)
	m := testModel(t)
	m.dlg = newMenuDialog("Commands", commandMenuItems())
	for i, it := range m.dlg.items {
		if it.action == "theme" {
			m.dlg.sel = i
		}
	}
	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.dlg == nil || m.dlg.title != "Theme" {
		t.Fatalf("expected the theme submenu to open, got %+v", m.dlg)
	}
	for i, it := range m.dlg.items {
		if it.action == "theme_opencode" {
			m.dlg.sel = i
		}
	}
	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyEnter})
	if activeTheme != "opencode" {
		t.Fatalf("activeTheme = %q, want opencode", activeTheme)
	}
	if m.dlg != nil {
		t.Error("theme submenu should close after a choice")
	}

	// Quitting must persist the choice so it survives a restart.
	m.quitCmd()
	got := loadConfig()
	if got.Theme != "opencode" {
		t.Errorf("saved theme = %q, want opencode", got.Theme)
	}
}

func TestChmodReportsSuccessCount(t *testing.T) {
	m := testModel(t)
	p := m.panels[m.active]
	p.selected["one.txt"] = true
	m, _ = m.doAction("chmod", "644")
	if m.statusErr {
		t.Errorf("chmod succeeded but statusErr is set: %q", m.status)
	}
	if !strings.Contains(m.status, "attributes changed") {
		t.Errorf("status = %q, want an 'attributes changed' message", m.status)
	}
}

// chmod on a name that no longer exists on disk (e.g. removed by another
// process between selection and the F11 dialog) must be reported, not
// silently swallowed the way the original implementation did.
func TestChmodReportsFailureWhenATargetIsGone(t *testing.T) {
	m := testModel(t)
	p := m.panels[m.active]
	p.selected["one.txt"] = true
	p.entries = append(p.entries, entry{name: "ghost.txt"})
	p.selected["ghost.txt"] = true

	m, _ = m.doAction("chmod", "644")
	if !m.statusErr {
		t.Errorf("a partially failed chmod should set statusErr; status = %q", m.status)
	}
	if !strings.Contains(m.status, "1/2 failed") {
		t.Errorf("status = %q, want it to report 1/2 failed", m.status)
	}
}

func TestChmodBadModeIsRejected(t *testing.T) {
	m := testModel(t)
	m.panels[m.active].selected["one.txt"] = true
	m, _ = m.doAction("chmod", "not-octal")
	if !m.statusErr || m.status != "bad mode" {
		t.Errorf("status = %q (err=%v), want \"bad mode\"", m.status, m.statusErr)
	}
}

func TestCdDistinguishesPermissionFromMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission semantics differ on windows")
	}
	m := testModel(t)
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	target := filepath.Join(locked, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)

	m, _ = m.runCommand("cd " + target)
	if !m.statusErr {
		t.Errorf("cd into an inaccessible directory should set statusErr; status = %q", m.status)
	}
	if strings.Contains(m.status, "no such directory") {
		t.Errorf("a permission error should not be reported as 'no such directory': %q", m.status)
	}
}

func TestCdReportsTrulyMissingDirectory(t *testing.T) {
	m := testModel(t)
	m, _ = m.runCommand("cd /this/does/not/exist")
	if !m.statusErr || !strings.Contains(m.status, "no such directory") {
		t.Errorf("status = %q, want a 'no such directory' error", m.status)
	}
}

func TestF8DefaultsToYesAndDeletesOnEnter(t *testing.T) {
	m := testModel(t)
	dir := m.panels[m.active].path
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyF8})
	if m.dlg == nil || m.dlg.kind != dlgConfirm || m.dlg.sel != 0 {
		t.Fatalf("F8 should open a confirm dialog defaulting to Yes (sel=0), got %+v", m.dlg)
	}
	m, cmd := m.handleDlgKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.dlg != nil {
		t.Error("dialog should close after Enter")
	}
	if !m.opRun {
		t.Fatal("F8 -> Enter (default Yes) should have started the delete operation")
	}
	m = pump(t, m, cmd, "")
	if _, err := os.Stat(filepath.Join(dir, "one.txt")); !os.IsNotExist(err) {
		t.Errorf("one.txt should have been deleted, stat err = %v", err)
	}
}

func TestCapitalDDefaultsToNoAndDoesNotDeleteOnEnter(t *testing.T) {
	m := testModel(t)
	p := m.panels[m.active]
	m, _ = m.handleKey(key('D'))
	if m.dlg == nil || m.dlg.kind != dlgConfirm || m.dlg.sel != 1 {
		t.Fatalf("D should open a confirm dialog defaulting to No (sel=1), got %+v", m.dlg)
	}
	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.dlg != nil {
		t.Error("dialog should close after Enter")
	}
	if m.opRun {
		t.Error("Enter on the No-default confirm should not start a delete")
	}
	if !p.has("one.txt") {
		t.Error("file was deleted despite the default (No) being accepted")
	}
}

func TestCapitalDCanBeConfirmedByMovingToYes(t *testing.T) {
	m := testModel(t)
	m, _ = m.handleKey(key('D'))
	if m.dlg.sel != 1 {
		t.Fatalf("setup: expected default sel=1 (No), got %d", m.dlg.sel)
	}
	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyLeft})
	if m.dlg.sel != 0 {
		t.Fatalf("Left should toggle the selection to Yes (0), got %d", m.dlg.sel)
	}
	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.opRun {
		t.Error("confirming Yes after toggling should start the delete operation")
	}
}

func TestCapitalDEscCancelsRegardlessOfSelection(t *testing.T) {
	m := testModel(t)
	m, _ = m.handleKey(key('D'))
	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyLeft}) // move to Yes
	m, _ = m.handleDlgKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.dlg != nil {
		t.Error("Esc should close the dialog")
	}
	if m.opRun {
		t.Error("Esc should never start the delete, even after moving to Yes")
	}
}

// The whole point of gating on cmdEmpty: composing an ordinary command that
// happens to contain a capital D must not be hijacked into a delete prompt.
func TestCapitalDDoesNotHijackCommandLineTyping(t *testing.T) {
	m := testModel(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.panels[m.active].path = dir

	for _, r := range "cd D" {
		m, _ = m.handleKey(key(r))
	}
	if m.dlg != nil {
		t.Fatalf("capital D mid-command should not open a dialog, got %+v", m.dlg)
	}
	if got := string(m.cmdline.text); got != "cd D" {
		t.Fatalf("command line = %q, want %q", got, "cd D")
	}
	for _, r := range "ownloads" {
		m, _ = m.handleKey(key(r))
	}
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.panels[m.active].path != filepath.Join(dir, "Downloads") {
		t.Errorf("path = %q, want the command to have run normally", m.panels[m.active].path)
	}
}

func quickViewModel(t *testing.T) model {
	t.Helper()
	t.Setenv("NC_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "content of a")
	write(t, filepath.Join(dir, "b.txt"), "content of b")
	m := newModel(dir, dir, cfg{})
	m.width, m.height = 100, 12
	m.panels[0].setCursor(1) // first real entry after ".."
	return m
}

func TestCapitalVTurnsOnQuickViewAndForcesLeftActive(t *testing.T) {
	m := quickViewModel(t)
	m.active = 1 // start on the right, to prove toggling forces it back
	m, _ = m.handleKey(key('V'))
	if !m.panels[1].quickView {
		t.Fatal("V should turn on quick view on the right panel")
	}
	if m.active != 0 {
		t.Errorf("active = %d, want 0 (left) after enabling quick view", m.active)
	}
	if !strings.Contains(m.View(), "content of a") {
		t.Error("the right pane should render the left panel's cursor file content")
	}
}

func TestCapitalVTogglesOff(t *testing.T) {
	m := quickViewModel(t)
	m, _ = m.handleKey(key('V'))
	if !m.panels[1].quickView {
		t.Fatal("setup: expected quick view on")
	}
	m, _ = m.handleKey(key('V'))
	if m.panels[1].quickView {
		t.Error("a second V should turn quick view back off")
	}
}

func TestQuickViewRejectsADirectoryCursor(t *testing.T) {
	m := quickViewModel(t)
	m.panels[0].setCursor(0) // ".."
	m, _ = m.handleKey(key('V'))
	if m.panels[1].quickView {
		t.Error("quick view should not turn on when the left cursor is on a directory")
	}
	if !m.statusErr {
		t.Errorf("status = %q, want an error", m.status)
	}
}

func TestQuickViewLiveUpdatesAsLeftCursorMoves(t *testing.T) {
	m := quickViewModel(t)
	m, _ = m.handleKey(key('V'))
	if !strings.Contains(m.View(), "content of a") {
		t.Fatal("setup: expected to see a.txt's content first")
	}
	m.panels[0].move(1) // to b.txt
	view := m.View()
	if strings.Contains(view, "content of a") {
		t.Error("quick view should have moved on from a.txt's content")
	}
	if !strings.Contains(view, "content of b") {
		t.Error("quick view should now show b.txt's content")
	}
}

func TestTabCannotFocusAQuickViewPane(t *testing.T) {
	m := quickViewModel(t)
	m, _ = m.handleKey(key('V'))
	if m.active != 0 {
		t.Fatalf("setup: expected active=0, got %d", m.active)
	}
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.active != 0 {
		t.Errorf("Tab should not switch onto a quick-view pane; active = %d", m.active)
	}
}

func TestEscExitsQuickView(t *testing.T) {
	m := quickViewModel(t)
	m, _ = m.handleKey(key('V'))
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.panels[1].quickView {
		t.Error("Esc should exit quick view")
	}
}

func TestSwapPanelsCancelsQuickView(t *testing.T) {
	m := quickViewModel(t)
	m, _ = m.handleKey(key('V'))
	if !m.panels[1].quickView {
		t.Fatal("setup: expected quick view on")
	}
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.panels[0].quickView || m.panels[1].quickView {
		t.Error("swapping panels should cancel quick view on both sides")
	}
}

// Same gating rule as capital D: composing a command that happens to contain
// a capital V must not be hijacked.
func TestCapitalVDoesNotHijackCommandLineTyping(t *testing.T) {
	m := quickViewModel(t)
	for _, r := range "git V" {
		m, _ = m.handleKey(key(r))
	}
	if m.panels[1].quickView {
		t.Error("capital V mid-command should not toggle quick view")
	}
	if got := string(m.cmdline.text); got != "git V" {
		t.Errorf("command line = %q, want %q", got, "git V")
	}
}

func TestPanelWidthsSplitEvenlyByDefault(t *testing.T) {
	m := testModel(t)
	l, r := m.panelWidths(101) // usable = 100
	if l != 50 || r != 50 {
		t.Errorf("panelWidths(101) = (%d, %d), want (50, 50)", l, r)
	}
}

func TestPanelWidthsFavorRightDuringQuickView(t *testing.T) {
	m := testModel(t)
	m.panels[1].quickView = true
	l, r := m.panelWidths(101) // usable = 100
	if r <= l {
		t.Errorf("panelWidths during quick view = (%d, %d), want right substantially larger", l, r)
	}
	if r < 60 {
		t.Errorf("right = %d, want the previewed file to get most of the width", r)
	}
}

// leftW+rightW must always equal width-1 (the divider column) — View()
// relies on this rather than padding to paper over a gap.
func TestPanelWidthsAlwaysFillTheAvailableWidth(t *testing.T) {
	m := testModel(t)
	for _, quickView := range []bool{false, true} {
		m.panels[1].quickView = quickView
		for _, w := range []int{40, 41, 60, 79, 80, 81, 120, 250} {
			l, r := m.panelWidths(w)
			if l+r != w-1 {
				t.Errorf("quickView=%v panelWidths(%d) = (%d, %d), sum %d, want %d", quickView, w, l, r, l+r, w-1)
			}
			if l < 1 || r < 1 {
				t.Errorf("quickView=%v panelWidths(%d) = (%d, %d), want both positive", quickView, w, l, r)
			}
		}
	}
}

func TestPanelWidthsKeepsLeftLegibleAtMinimumWidth(t *testing.T) {
	m := testModel(t)
	m.panels[1].quickView = true
	l, _ := m.panelWidths(40) // the app's enforced minimum
	if l < 10 {
		t.Errorf("left width at minimum terminal size = %d, too narrow to show filenames", l)
	}
}

func TestQuickViewWidensTheRightPanelInTheRenderedFrame(t *testing.T) {
	withColor(t)
	m := quickViewModel(t)
	beforeL, beforeR := m.panelWidths(m.width)
	m, _ = m.handleKey(key('V'))
	afterL, afterR := m.panelWidths(m.width)
	if afterR <= beforeR {
		t.Errorf("right width after V = %d, want it larger than the pre-quick-view width %d", afterR, beforeR)
	}
	if afterL >= beforeL {
		t.Errorf("left width after V = %d, want it smaller than the pre-quick-view width %d", afterL, beforeL)
	}
	checkFrame(t, m.View(), m.width, m.height)
}
