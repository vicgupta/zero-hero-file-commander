package main

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Markdown highlighting for the viewer (F3) and Quick View (Shift+V). This is
// a line-based approximation, not a full CommonMark parser: block structure
// (heading/quote/list/rule/fence) is read one line at a time, and inline
// styling (bold/italic/code/link) is applied within whatever text remains.
// Good enough to make a README readable at a glance; not a spec-complete
// renderer.

var (
	reMdHeading = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reMdRule    = regexp.MustCompile(`^ {0,3}(?:-{3,}|\*{3,}|_{3,})\s*$`)
	reMdQuote   = regexp.MustCompile(`^(\s*>+)\s?(.*)$`)
	reMdList    = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])(\s+)(.*)$`)
	reMdFence   = regexp.MustCompile("^ {0,3}(```|~~~)")

	// One alternation so overlapping delimiters (e.g. "**" vs "*") resolve by
	// leftmost-first preference: the earlier branch wins at a shared start.
	reMdInline = regexp.MustCompile(
		"`[^`]+`" + `|\*\*[^*]+\*\*` + `|__[^_]+__` + `|\[[^\]]*\]\([^)]*\)` + `|\*[^*]+\*` + `|_[^_]+_`,
	)
)

// mdHighlightLines styles every source line, tracking fenced-code-block state
// across lines (an "inside a fence" line can't be judged on its own).
func mdHighlightLines(lines []string) []srow {
	out := make([]srow, len(lines))
	inFence := false
	for i, l := range lines {
		if reMdFence.MatchString(l) {
			out[i] = srow{{l, styleMdCodeBlock}}
			inFence = !inFence
			continue
		}
		if inFence {
			out[i] = srow{{l, styleMdCodeBlock}}
			continue
		}
		out[i] = mdHighlightLine(l)
	}
	return out
}

// mdHighlightLine styles one line's block-level construct (heading, rule,
// blockquote, list item, or plain paragraph) and then its inline spans.
func mdHighlightLine(l string) srow {
	switch {
	case reMdRule.MatchString(l):
		return srow{{l, styleMdRule}}
	case reMdHeading.MatchString(l):
		m := reMdHeading.FindStringSubmatch(l)
		st := styleMdH2
		switch len(m[1]) {
		case 1:
			st = styleMdH1
		case 2:
			st = styleMdH2
		default:
			st = styleMdH3
		}
		row := srow{{m[1] + " ", st}}
		return append(row, mdInlineSpans(m[2], st)...)
	case reMdQuote.MatchString(l):
		m := reMdQuote.FindStringSubmatch(l)
		row := srow{{m[1] + " ", styleMdQuote}}
		return append(row, mdInlineSpans(m[2], styleMdQuote)...)
	case reMdList.MatchString(l):
		m := reMdList.FindStringSubmatch(l)
		row := srow{{m[1] + m[2] + m[3], styleMdBullet}}
		return append(row, mdInlineSpans(m[4], styleViewer)...)
	default:
		return mdInlineSpans(l, styleViewer)
	}
}

// mdInlineSpans splits s on inline markdown delimiters (code, bold, italic,
// links), styling each token and leaving the rest as base-styled plain text.
func mdInlineSpans(s string, base lipgloss.Style) srow {
	idxs := reMdInline.FindAllStringIndex(s, -1)
	if len(idxs) == 0 {
		return srow{{s, base}}
	}
	var out srow
	pos := 0
	for _, m := range idxs {
		if m[0] > pos {
			out = append(out, span{s[pos:m[0]], base})
		}
		out = append(out, mdInlineToken(s[m[0]:m[1]], base)...)
		pos = m[1]
	}
	if pos < len(s) {
		out = append(out, span{s[pos:], base})
	}
	return out
}

func mdInlineToken(tok string, base lipgloss.Style) srow {
	switch {
	case strings.HasPrefix(tok, "`"):
		return srow{{strings.Trim(tok, "`"), styleMdCode}}
	case strings.HasPrefix(tok, "**"):
		return srow{{strings.Trim(tok, "*"), styleMdBold}}
	case strings.HasPrefix(tok, "__"):
		return srow{{strings.Trim(tok, "_"), styleMdBold}}
	case strings.HasPrefix(tok, "["):
		close := strings.Index(tok, "](")
		if close == -1 {
			return srow{{tok, base}}
		}
		text, url := tok[1:close], tok[close+2:len(tok)-1]
		out := srow{{text, styleMdLinkText}}
		if url != "" {
			out = append(out, span{" (" + url + ")", styleMdLinkURL})
		}
		return out
	case strings.HasPrefix(tok, "*"):
		return srow{{strings.Trim(tok, "*"), styleMdItalic}}
	case strings.HasPrefix(tok, "_"):
		return srow{{strings.Trim(tok, "_"), styleMdItalic}}
	default:
		return srow{{tok, base}}
	}
}

// isMarkdownPath reports whether path's extension marks it as markdown.
func isMarkdownPath(path string) bool {
	ext := strings.ToLower(path)
	if i := strings.LastIndexByte(ext, '.'); i >= 0 {
		ext = ext[i:]
	} else {
		return false
	}
	return ext == ".md" || ext == ".markdown" || ext == ".mdown"
}
