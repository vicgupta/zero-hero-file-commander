package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// opMsg is a progress/final message from a background file operation.
type opMsg struct {
	label string
	done  bool
	err   error
	count int
}

// Overwrite policy for a running operation (spec §9.1.2).
const (
	owAsk = iota
	owOverwriteAll
	owSkipAll
)

// Answers the user can give to a collision prompt.
const (
	ansOverwrite = iota
	ansSkip
	ansCancel
)

type conflictAnswer struct {
	action int
	all    bool // apply to every remaining collision
}

// conflict describes a destination that already exists and is about to be
// clobbered. The operation goroutine blocks on reply until the UI answers.
type conflict struct {
	src, dst         string
	srcSize, dstSize int64
	srcMod, dstMod   time.Time
	dstDir           bool
	reply            chan conflictAnswer
}

// conflictMsg hands a pending collision to the UI.
type conflictMsg struct{ c *conflict }

var (
	errSameFile  = errors.New("source and destination are the same file")
	errIntoSelf  = errors.New("cannot copy a directory into itself")
	errCancelled = errors.New("cancelled")
)

// runner carries the shared state of one background file operation.
type runner struct {
	ctx    context.Context
	ch     chan tea.Msg
	policy int
}

// sameTarget reports whether src and dst are the same file. Both the entries
// themselves and whatever they resolve to are compared, so a symlink or hard
// link cannot be used to write over the source through an alias.
func sameTarget(src, dst string) bool {
	if si, err := os.Lstat(src); err == nil {
		if di, err := os.Lstat(dst); err == nil && os.SameFile(si, di) {
			return true
		}
	}
	si, serr := os.Stat(src)
	di, derr := os.Stat(dst)
	return serr == nil && derr == nil && os.SameFile(si, di)
}

// decide reports whether dst may be written, resolving collisions against the
// operation's overwrite policy and asking the user when the policy is "ask".
// A false return with a nil error means "skip this item".
func (r *runner) decide(src, dst string) (bool, error) {
	di, err := os.Lstat(dst)
	if err != nil {
		return true, nil // nothing there: nothing to clobber
	}
	// Copying or moving a file onto itself would truncate it. Never proceed.
	if sameTarget(src, dst) {
		return false, errSameFile
	}
	switch r.policy {
	case owOverwriteAll:
		return true, nil
	case owSkipAll:
		return false, nil
	}

	c := &conflict{src: src, dst: dst, reply: make(chan conflictAnswer, 1)}
	c.dstSize, c.dstMod, c.dstDir = di.Size(), di.ModTime(), di.IsDir()
	if si, err := os.Lstat(src); err == nil {
		c.srcSize, c.srcMod = si.Size(), si.ModTime()
	}
	select {
	case r.ch <- conflictMsg{c}:
	case <-r.ctx.Done():
		return false, errCancelled
	}
	select {
	case a := <-c.reply:
		if a.all {
			switch a.action {
			case ansOverwrite:
				r.policy = owOverwriteAll
			case ansSkip:
				r.policy = owSkipAll
			}
		}
		switch a.action {
		case ansOverwrite:
			return true, nil
		case ansSkip:
			return false, nil
		default:
			return false, errCancelled
		}
	case <-r.ctx.Done():
		return false, errCancelled
	}
}

// withinDir reports whether child is parent itself or lies inside it.
func withinDir(parent, child string) bool {
	p, c := filepath.Clean(parent), filepath.Clean(child)
	return p == c || strings.HasPrefix(c, p+string(filepath.Separator))
}

func (r *runner) copyPath(src, dst string, isDir bool) error {
	if r.ctx.Err() != nil {
		return errCancelled
	}
	if st, err := os.Lstat(src); err == nil && st.Mode()&os.ModeSymlink != 0 {
		ok, err := r.decide(src, dst)
		if err != nil || !ok {
			return err
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	if isDir {
		// Recursing into the copy we are writing would never terminate.
		if withinDir(src, dst) {
			return errIntoSelf
		}
		if di, err := os.Lstat(dst); err == nil && !di.IsDir() {
			ok, derr := r.decide(src, dst)
			if derr != nil || !ok {
				return derr
			}
			if err := os.Remove(dst); err != nil {
				return err
			}
		}
		perm := os.FileMode(0o755)
		if si, err := os.Stat(src); err == nil {
			perm = si.Mode().Perm()
		}
		if err := os.MkdirAll(dst, perm); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if r.ctx.Err() != nil {
				return errCancelled
			}
			if err := r.copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()), e.IsDir()); err != nil {
				return err
			}
		}
		return nil
	}
	ok, err := r.decide(src, dst)
	if err != nil || !ok {
		return err
	}
	return r.copyFile(src, dst)
}

// copyFile writes through a sibling temp file and renames it into place, so an
// interrupted or failed copy can never truncate an existing destination.
func (r *runner) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.CreateTemp(filepath.Dir(dst), ".nc-part-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	done := false
	defer func() {
		if !done {
			out.Close()
			os.Remove(tmp)
		}
	}()
	if err := out.Chmod(st.Mode().Perm()); err != nil {
		return err
	}

	buf := make([]byte, 256<<10)
	for {
		if r.ctx.Err() != nil {
			return errCancelled
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chtimes(tmp, st.ModTime(), st.ModTime()); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	done = true
	return nil
}

func (r *runner) movePath(src, dst string, isDir bool) error {
	if r.ctx.Err() != nil {
		return errCancelled
	}
	if isDir && withinDir(src, dst) {
		return errIntoSelf
	}
	if _, err := os.Lstat(dst); err == nil {
		if sameTarget(src, dst) {
			return errSameFile
		}
		ok, derr := r.decide(src, dst)
		if derr != nil || !ok {
			return derr
		}
		// Rename will not replace a non-empty directory, so clear the way.
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := r.copyPath(src, dst, isDir); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func (r *runner) deletePath(path string, isDir bool) error {
	if r.ctx.Err() != nil {
		return errCancelled
	}
	if isDir {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

// checkDestDir rejects a target that is missing or is not a directory, rather
// than discovering it item by item mid-operation.
func checkDestDir(dir string) error {
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("cannot access destination: %s", describeErr(err))
		}
		return fmt.Errorf("destination does not exist: %s", dir)
	}
	if !st.IsDir() {
		return fmt.Errorf("destination is not a directory: %s", dir)
	}
	return nil
}

// startOp launches fn over items in a goroutine, reporting progress on m.opCh.
//
// Exactly one opSub() receiver is outstanding while an operation runs, so the
// plain channel sends below always find a reader. The one place that invariant
// is suspended is a pending conflict, where the UI deliberately stops receiving
// until the user answers — hence the ctx-guarded send in decide.
func (m model) startOp(verb string, items []entry, fn func(*runner, entry) error) (model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &runner{ctx: ctx, ch: m.opCh}
	m.opRun = true
	m.opCancel = cancel
	m.status = verb + "..."
	m.statusErr = false

	ch := m.opCh
	go func() {
		defer cancel()
		n := 0
		for i, it := range items {
			if ctx.Err() != nil {
				ch <- opMsg{label: verb + " cancelled", done: true, err: errCancelled}
				return
			}
			ch <- opMsg{label: fmt.Sprintf("%s %s (%d/%d)", verb, it.name, i+1, len(items))}
			if err := fn(r, it); err != nil {
				label := verb + " failed: " + describeErr(err)
				if errors.Is(err, errCancelled) {
					label = verb + " cancelled"
				}
				ch <- opMsg{label: label, done: true, err: err}
				return
			}
			n++
		}
		ch <- opMsg{label: fmt.Sprintf("%s done: %d item(s)", verb, n), done: true, count: n}
	}()
	return m, m.opSub()
}

func (m model) startCopy(srcDir, dstDir string, items []entry) (model, tea.Cmd) {
	if len(items) == 0 {
		return m.fail("nothing to copy")
	}
	if err := checkDestDir(dstDir); err != nil {
		return m.fail("Copy: " + describeErr(err))
	}
	return m.startOp("Copy", items, func(r *runner, it entry) error {
		return r.copyPath(filepath.Join(srcDir, it.name), filepath.Join(dstDir, it.name), it.dir)
	})
}

func (m model) startMove(srcDir, dstDir string, items []entry) (model, tea.Cmd) {
	if len(items) == 0 {
		return m.fail("nothing to move")
	}
	if err := checkDestDir(dstDir); err != nil {
		return m.fail("Move: " + describeErr(err))
	}
	return m.startOp("Move", items, func(r *runner, it entry) error {
		return r.movePath(filepath.Join(srcDir, it.name), filepath.Join(dstDir, it.name), it.dir)
	})
}

func (m model) startDelete(dir string, items []entry) (model, tea.Cmd) {
	if len(items) == 0 {
		return m.fail("nothing to delete")
	}
	return m.startOp("Delete", items, func(r *runner, it entry) error {
		return r.deletePath(filepath.Join(dir, it.name), it.dir)
	})
}

// opSub receives the next message from the running operation.
func (m model) opSub() tea.Cmd {
	return func() tea.Msg { return <-m.opCh }
}
