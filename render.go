package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// span is a run of text drawn with a single style. The text is always plain:
// it never contains ANSI escapes. All layout arithmetic happens on spans, and
// styling is applied exactly once, when the row is finally rendered. Padding or
// truncating an already-styled string counts escape bytes as visible cells and
// shreds the layout, so nothing outside this file may do it.
type span struct {
	text  string
	style lipgloss.Style
}

// srow is one screen row: spans laid left to right.
type srow []span

// width is the row's visible width in cells.
func (r srow) width() int {
	n := 0
	for _, s := range r {
		n += len([]rune(s.text))
	}
	return n
}

// plain returns the row's text with no styling applied.
func (r srow) plain() string {
	var sb strings.Builder
	for _, s := range r {
		sb.WriteString(s.text)
	}
	return sb.String()
}

// String renders the row, applying each span's style to its own text.
func (r srow) String() string {
	var sb strings.Builder
	for _, s := range r {
		sb.WriteString(s.style.Render(s.text))
	}
	return sb.String()
}

// pad returns the row adjusted to exactly w cells: padded with st-styled blanks
// if short, truncated if long.
func (r srow) pad(w int, st lipgloss.Style) srow {
	n := r.width()
	switch {
	case n == w:
		return r
	case n > w:
		head, _ := r.splitAt(w)
		return head
	}
	out := make(srow, len(r), len(r)+1)
	copy(out, r)
	return append(out, span{strings.Repeat(" ", w-n), st})
}

// splitAt divides the row at visible column c, splitting a span if c falls
// inside one.
func (r srow) splitAt(c int) (srow, srow) {
	var head, tail srow
	col := 0
	for _, s := range r {
		n := len([]rune(s.text))
		switch {
		case col >= c:
			tail = append(tail, s)
		case col+n <= c:
			head = append(head, s)
		default:
			rs := []rune(s.text)
			cut := c - col
			head = append(head, span{string(rs[:cut]), s.style})
			tail = append(tail, span{string(rs[cut:]), s.style})
		}
		col += n
	}
	return head, tail
}

// overlay replaces the visible columns [x, x+sub.width()) with sub, keeping the
// row's total width unchanged.
func (r srow) overlay(x int, sub srow) srow {
	head, _ := r.splitAt(x)
	_, tail := r.splitAt(x + sub.width())
	out := make(srow, 0, len(head)+len(sub)+len(tail))
	out = append(out, head...)
	out = append(out, sub...)
	return append(out, tail...)
}

// renderRows joins rows into the final frame.
func renderRows(rows []srow) string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.String()
	}
	return strings.Join(out, "\n")
}
