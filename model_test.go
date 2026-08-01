package main

import (
	"os"
	"path/filepath"
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

func TestVOpensTheViewerOnAFileJustLikeEnter(t *testing.T) {
	m := viModelWithSubdir(t)
	p := m.panels[m.active]
	for i, e := range p.entries {
		if e.name == "a.txt" {
			p.setCursor(i)
		}
	}
	m, _ = m.handleKey(key('v'))
	if m.dlg == nil || m.dlg.kind != dlgViewer {
		t.Fatalf("v on a file should open the viewer, got %+v", m.dlg)
	}
}

func TestVEntersADirectoryInsteadOfViewingIt(t *testing.T) {
	m := viModelWithSubdir(t)
	p := m.panels[m.active]
	for i, e := range p.entries {
		if e.name == "sub" {
			p.setCursor(i)
		}
	}
	start := p.path
	m, _ = m.handleKey(key('v'))
	if p.path != filepath.Join(start, "sub") {
		t.Fatalf("v: path = %q, want entering sub", p.path)
	}
	if m.dlg != nil {
		t.Error("v on a directory should enter it, not open the viewer")
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
