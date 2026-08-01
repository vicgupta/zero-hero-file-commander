package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestRunner returns a runner backed by a fake UI that answers collision
// prompts from answers in order, repeating the last one. With no answers given,
// any prompt fails the test: the operation was expected not to hit a conflict.
func newTestRunner(t *testing.T, answers ...conflictAnswer) (*runner, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &runner{ctx: ctx, ch: make(chan tea.Msg)}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case msg := <-r.ch:
				cm, ok := msg.(conflictMsg)
				if !ok {
					continue
				}
				if len(answers) == 0 {
					t.Errorf("unexpected overwrite prompt for %s", cm.c.dst)
					cm.c.reply <- conflictAnswer{action: ansCancel}
					continue
				}
				a := answers[len(answers)-1]
				if i < len(answers) {
					a = answers[i]
				}
				cm.c.reply <- a
			}
		}
	}()
	t.Cleanup(func() { cancel(); <-stopped })
	return r, cancel
}

func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// noTempLeftovers guards against a half-written copy being left behind.
func noTempLeftovers(t *testing.T, dir string) {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(dir, ".nc-part-*"))
	if len(matches) > 0 {
		t.Errorf("temp files left behind: %v", matches)
	}
}

func TestCopyFilePreservesContentModeAndTime(t *testing.T) {
	r, _ := newTestRunner(t)
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "src.txt"), "hello")
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(src, when, when); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst.txt")
	if err := r.copyPath(src, dst, false); err != nil {
		t.Fatalf("copyPath: %v", err)
	}
	if got := read(t, dst); got != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", st.Mode().Perm())
	}
	if !st.ModTime().Truncate(time.Second).Equal(when) {
		t.Errorf("mtime = %v, want %v", st.ModTime(), when)
	}
	noTempLeftovers(t, dir)
}

// The shipped bug: O_TRUNC on the destination emptied the file before reading it.
func TestCopyOntoItselfIsRefusedAndLeavesFileIntact(t *testing.T) {
	r, _ := newTestRunner(t)
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "data.txt"), "important contents")

	err := r.copyPath(src, src, false)
	if !errors.Is(err, errSameFile) {
		t.Fatalf("copyPath onto itself err = %v, want errSameFile", err)
	}
	if got := read(t, src); got != "important contents" {
		t.Fatalf("file was damaged: %q", got)
	}
	noTempLeftovers(t, dir)
}

func TestCopyThroughSymlinkToItselfIsRefused(t *testing.T) {
	r, _ := newTestRunner(t)
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "data.txt"), "payload")
	link := filepath.Join(dir, "alias.txt")
	if err := os.Symlink(src, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Copying the real file over a symlink that resolves to it must not wipe it.
	if err := r.copyPath(src, link, false); !errors.Is(err, errSameFile) {
		t.Fatalf("err = %v, want errSameFile", err)
	}
	if got := read(t, src); got != "payload" {
		t.Fatalf("file was damaged: %q", got)
	}
}

func TestCopyNeverClobbersWithoutAnAnswer(t *testing.T) {
	for _, tc := range []struct {
		name string
		ans  conflictAnswer
		want string
		err  error
	}{
		{"skip", conflictAnswer{action: ansSkip}, "original", nil},
		{"overwrite", conflictAnswer{action: ansOverwrite}, "replacement", nil},
		{"cancel", conflictAnswer{action: ansCancel}, "original", errCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newTestRunner(t, tc.ans)
			dir := t.TempDir()
			src := write(t, filepath.Join(dir, "src.txt"), "replacement")
			dst := write(t, filepath.Join(dir, "dst.txt"), "original")

			err := r.copyPath(src, dst, false)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if got := read(t, dst); got != tc.want {
				t.Errorf("destination = %q, want %q", got, tc.want)
			}
			noTempLeftovers(t, dir)
		})
	}
}

func TestCopyOverwriteAllStopsAsking(t *testing.T) {
	// Only one answer is supplied; "all" must cover the remaining collisions.
	r, _ := newTestRunner(t, conflictAnswer{action: ansOverwrite, all: true})
	src, dst := t.TempDir(), t.TempDir()
	names := []string{"a.txt", "b.txt", "c.txt"}
	for _, n := range names {
		write(t, filepath.Join(src, n), "new")
		write(t, filepath.Join(dst, n), "old")
	}
	for _, n := range names {
		if err := r.copyPath(filepath.Join(src, n), filepath.Join(dst, n), false); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	for _, n := range names {
		if got := read(t, filepath.Join(dst, n)); got != "new" {
			t.Errorf("%s = %q, want %q", n, got, "new")
		}
	}
	if r.policy != owOverwriteAll {
		t.Errorf("policy = %d, want owOverwriteAll", r.policy)
	}
}

func TestCopySkipAllStopsAsking(t *testing.T) {
	r, _ := newTestRunner(t, conflictAnswer{action: ansSkip, all: true})
	src, dst := t.TempDir(), t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		write(t, filepath.Join(src, n), "new")
		write(t, filepath.Join(dst, n), "old")
		if err := r.copyPath(filepath.Join(src, n), filepath.Join(dst, n), false); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if got := read(t, filepath.Join(dst, n)); got != "old" {
			t.Errorf("%s = %q, want it untouched", n, got)
		}
	}
}

func TestCopyDirectoryRecursively(t *testing.T) {
	r, _ := newTestRunner(t)
	root := t.TempDir()
	src := filepath.Join(root, "src")
	os.MkdirAll(filepath.Join(src, "nested", "deep"), 0o755)
	write(t, filepath.Join(src, "top.txt"), "top")
	write(t, filepath.Join(src, "nested", "mid.txt"), "mid")
	write(t, filepath.Join(src, "nested", "deep", "low.txt"), "low")

	dst := filepath.Join(root, "dst")
	if err := r.copyPath(src, dst, true); err != nil {
		t.Fatalf("copyPath: %v", err)
	}
	for rel, want := range map[string]string{
		"top.txt":             "top",
		"nested/mid.txt":      "mid",
		"nested/deep/low.txt": "low",
	} {
		if got := read(t, filepath.Join(dst, filepath.FromSlash(rel))); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestCopyDirectoryIntoItselfIsRefused(t *testing.T) {
	r, _ := newTestRunner(t)
	root := t.TempDir()
	src := filepath.Join(root, "src")
	os.MkdirAll(src, 0o755)
	write(t, filepath.Join(src, "f.txt"), "x")

	// Both the directory itself and a path beneath it would recurse forever.
	for _, dst := range []string{src, filepath.Join(src, "copy")} {
		if err := r.copyPath(src, dst, true); !errors.Is(err, errIntoSelf) {
			t.Errorf("copyPath(%s -> %s) err = %v, want errIntoSelf", src, dst, err)
		}
	}
}

func TestCopySymlinkStaysALink(t *testing.T) {
	r, _ := newTestRunner(t)
	dir := t.TempDir()
	target := write(t, filepath.Join(dir, "target.txt"), "data")
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dst := filepath.Join(dir, "copy.txt")
	if err := r.copyPath(link, dst, false); err != nil {
		t.Fatalf("copyPath: %v", err)
	}
	st, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was dereferenced instead of copied as a link")
	}
}

// Cancelling must never leave the destination truncated — the reason copies go
// through a temp file and a rename.
func TestCancelledCopyLeavesDestinationUntouched(t *testing.T) {
	r, cancel := newTestRunner(t, conflictAnswer{action: ansOverwrite})
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "src.txt"), strings.Repeat("n", 1<<20))
	dst := write(t, filepath.Join(dir, "dst.txt"), "original contents")

	cancel()
	if err := r.copyFile(src, dst); !errors.Is(err, errCancelled) {
		t.Fatalf("err = %v, want errCancelled", err)
	}
	if got := read(t, dst); got != "original contents" {
		t.Fatalf("destination was damaged: %q", got)
	}
	noTempLeftovers(t, dir)
}

func TestMoveWithinDirectoryRenames(t *testing.T) {
	r, _ := newTestRunner(t)
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "old.txt"), "body")
	dst := filepath.Join(dir, "new.txt")
	if err := r.movePath(src, dst, false); err != nil {
		t.Fatalf("movePath: %v", err)
	}
	if got := read(t, dst); got != "body" {
		t.Errorf("content = %q", got)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Error("source still present after move")
	}
}

func TestMoveOntoItselfIsRefused(t *testing.T) {
	r, _ := newTestRunner(t)
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "f.txt"), "keep me")
	if err := r.movePath(src, src, false); !errors.Is(err, errSameFile) {
		t.Fatalf("err = %v, want errSameFile", err)
	}
	if got := read(t, src); got != "keep me" {
		t.Fatalf("file was damaged: %q", got)
	}
}

func TestMoveRespectsOverwriteAnswer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ans     conflictAnswer
		wantDst string
		srcGone bool
	}{
		{"skip", conflictAnswer{action: ansSkip}, "old", false},
		{"overwrite", conflictAnswer{action: ansOverwrite}, "new", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newTestRunner(t, tc.ans)
			from, to := t.TempDir(), t.TempDir()
			src := write(t, filepath.Join(from, "f.txt"), "new")
			dst := write(t, filepath.Join(to, "f.txt"), "old")

			if err := r.movePath(src, dst, false); err != nil {
				t.Fatalf("movePath: %v", err)
			}
			if got := read(t, dst); got != tc.wantDst {
				t.Errorf("destination = %q, want %q", got, tc.wantDst)
			}
			_, err := os.Lstat(src)
			if gone := os.IsNotExist(err); gone != tc.srcGone {
				t.Errorf("source gone = %v, want %v", gone, tc.srcGone)
			}
		})
	}
}

func TestMoveDirectoryOntoExistingDirectory(t *testing.T) {
	r, _ := newTestRunner(t, conflictAnswer{action: ansOverwrite})
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	os.MkdirAll(src, 0o755)
	os.MkdirAll(dst, 0o755)
	write(t, filepath.Join(src, "new.txt"), "new")
	write(t, filepath.Join(dst, "stale.txt"), "stale")

	if err := r.movePath(src, dst, true); err != nil {
		t.Fatalf("movePath: %v", err)
	}
	if got := read(t, filepath.Join(dst, "new.txt")); got != "new" {
		t.Errorf("moved file = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dst, "stale.txt")); !os.IsNotExist(err) {
		t.Error("replaced directory should not retain its old contents")
	}
}

func TestDeleteFileAndDirectory(t *testing.T) {
	r, _ := newTestRunner(t)
	dir := t.TempDir()
	f := write(t, filepath.Join(dir, "f.txt"), "x")
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(filepath.Join(sub, "deep"), 0o755)
	write(t, filepath.Join(sub, "deep", "g.txt"), "y")

	if err := r.deletePath(f, false); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if err := r.deletePath(sub, true); err != nil {
		t.Fatalf("delete dir: %v", err)
	}
	for _, p := range []string{f, sub} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
}

func TestWithinDir(t *testing.T) {
	sep := string(filepath.Separator)
	for _, tc := range []struct {
		parent, child string
		want          bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/b/c/d", true},
		{"/a/b", "/a/bc", false},
		{"/a/b", "/a", false},
		{"/a/b", "/x", false},
		{"/a/b" + sep, "/a/b/c", true},
	} {
		if got := withinDir(tc.parent, tc.child); got != tc.want {
			t.Errorf("withinDir(%q, %q) = %v, want %v", tc.parent, tc.child, got, tc.want)
		}
	}
}

func TestCheckDestDir(t *testing.T) {
	dir := t.TempDir()
	if err := checkDestDir(dir); err != nil {
		t.Errorf("existing dir rejected: %v", err)
	}
	if err := checkDestDir(filepath.Join(dir, "nope")); err == nil {
		t.Error("missing destination should be rejected")
	}
	f := write(t, filepath.Join(dir, "f.txt"), "x")
	if err := checkDestDir(f); err == nil {
		t.Error("file destination should be rejected")
	}
}
