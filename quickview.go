package main

// renderQuickView draws a live preview of srcEntry (from the *other* panel's
// cursor) instead of a directory listing: a header naming the file, then its
// content from the top — no independent scrolling, matching the spec's
// "Quick View... viewer minimized in panel. Read-only." (§6.2). Content is
// re-read on every call rather than cached, so it always reflects whatever
// the source panel's cursor is currently on.
func renderQuickView(srcEntry entry, srcPath string, w, h int, active bool) []srow {
	rows := make([]srow, h)
	hdr := styleInactiveHeader
	if active {
		hdr = styleActiveHeader
	}
	rows[0] = srow{{padRune(" Quick View: "+quickViewLabel(srcEntry), w), hdr}}

	content := quickViewRows(srcEntry, srcPath, w-1)
	for i := 1; i < h; i++ {
		var row srow
		if n := i - 1; n < len(content) {
			row = content[n]
		}
		line := append(srow{{" ", styleViewer}}, row...)
		rows[i] = line.pad(w, styleViewer)
	}
	return rows
}

func quickViewLabel(e entry) string {
	if e.name == "" || e.name == ".." || e.dir {
		return "(none)"
	}
	return e.name
}

// quickViewPlaceholder reports the message to show instead of file content
// when there's no real file to preview (empty cursor, "..", a directory).
func quickViewPlaceholder(e entry) (string, bool) {
	switch {
	case e.name == "" || e.name == "..":
		return "(no file under the cursor)", true
	case e.dir:
		return "(" + e.name + " is a directory)", true
	}
	return "", false
}

func quickViewLines(e entry, path string) []string {
	if msg, ok := quickViewPlaceholder(e); ok {
		return []string{msg}
	}
	v, err := newViewer(path)
	if err != nil {
		return []string{"(cannot read: " + err.Error() + ")"}
	}
	return v.lines
}

// quickViewRows is quickViewLines' styled counterpart: markdown-highlighted,
// theme-colored rows for a real file, or a single placeholder row otherwise.
func quickViewRows(e entry, path string, w int) []srow {
	if msg, ok := quickViewPlaceholder(e); ok {
		return []srow{{{msg, styleViewer}}}
	}
	v, err := newViewer(path)
	if err != nil {
		return []srow{{{"(cannot read: " + err.Error() + ")", styleViewer}}}
	}
	return v.rows(w)
}
