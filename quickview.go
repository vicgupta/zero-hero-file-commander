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

	lines := quickViewLines(srcEntry, srcPath)
	for i := 1; i < h; i++ {
		var text string
		if n := i - 1; n < len(lines) {
			text = lines[n]
		}
		rows[i] = srow{{padRune(" "+truncRune(text, w-1), w), styleViewer}}
	}
	return rows
}

func quickViewLabel(e entry) string {
	if e.name == "" || e.name == ".." || e.dir {
		return "(none)"
	}
	return e.name
}

func quickViewLines(e entry, path string) []string {
	if e.name == "" || e.name == ".." {
		return []string{"(no file under the cursor)"}
	}
	if e.dir {
		return []string{"(" + e.name + " is a directory)"}
	}
	v, err := newViewer(path)
	if err != nil {
		return []string{"(cannot read: " + err.Error() + ")"}
	}
	return v.lines
}
