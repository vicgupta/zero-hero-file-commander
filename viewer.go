package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// maxViewBytes caps how much of a file the viewer reads.
const maxViewBytes = 4 << 20 // 4 MiB

// viewer is the F3 file viewer with text and hex modes.
type viewer struct {
	path     string
	data     []byte
	lines    []string
	hex      bool
	wrap     bool
	markdown bool

	// dispRows caches the styled, wrapped rows for the (width, hex, wrap)
	// they were built for, so scrolling (which re-renders every frame but
	// changes none of those three) doesn't re-run markdown highlighting or
	// word wrap on every keypress.
	dispWidth int
	dispHex   bool
	dispWrap  bool
	dispRows  []srow
}

func newViewer(path string) (*viewer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	n := st.Size()
	if n > maxViewBytes {
		n = maxViewBytes
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(f, buf); err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	// Word wrap defaults on for text/markdown so long lines are readable
	// without scrolling sideways; it's simply ignored once hex mode kicks in.
	v := &viewer{path: path, data: buf, markdown: isMarkdownPath(path), wrap: true}
	if looksBinary(buf) {
		v.hex = true
	}
	v.render()
	return v, nil
}

func looksBinary(b []byte) bool {
	n := len(b)
	if n > 512 {
		n = 512
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}

func (v *viewer) toggleHex() {
	v.hex = !v.hex
	v.render()
}

func (v *viewer) toggleWrap() {
	v.wrap = !v.wrap
}

func (v *viewer) render() {
	if v.hex {
		v.lines = hexDump(v.data)
	} else {
		v.lines = textLines(v.data)
	}
	v.dispRows = nil // invalidate the row cache; the source lines changed
}

// rows returns the viewer's content as styled, display-ready rows: markdown
// highlighted when the file looks like markdown, word-wrapped to w when wrap
// is on. Recomputed only when width/hex/wrap actually change.
func (v *viewer) rows(w int) []srow {
	if v.dispRows != nil && w == v.dispWidth && v.hex == v.dispHex && v.wrap == v.dispWrap {
		return v.dispRows
	}
	v.dispWidth, v.dispHex, v.dispWrap = w, v.hex, v.wrap

	var base []srow
	if !v.hex && v.markdown {
		base = mdHighlightLines(v.lines)
	} else {
		base = make([]srow, len(v.lines))
		for i, l := range v.lines {
			base[i] = srow{{l, styleViewer}}
		}
	}

	rows := base
	if v.wrap && !v.hex {
		rows = make([]srow, 0, len(base))
		for _, row := range base {
			rows = append(rows, wrapSrow(row, w)...)
		}
	}
	if len(rows) == 0 {
		rows = []srow{{}}
	}
	v.dispRows = rows
	return rows
}

// textLines splits data into sanitized lines (no escape/control injection).
func textLines(b []byte) []string {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))
	for _, l := range raw {
		var sb strings.Builder
		for _, r := range l {
			switch {
			case r == '\t':
				sb.WriteString("    ")
			case r < 0x20 || r == 0x7f:
				sb.WriteRune('?')
			default:
				sb.WriteRune(r)
			}
		}
		lines = append(lines, sb.String())
	}
	return lines
}

func hexDump(b []byte) []string {
	const per = 16
	lines := make([]string, 0, (len(b)+per-1)/per)
	for off := 0; off < len(b); off += per {
		end := off + per
		if end > len(b) {
			end = len(b)
		}
		chunk := b[off:end]
		var hex, ascii strings.Builder
		fmt.Fprintf(&hex, "%08x  ", off)
		for _, c := range chunk {
			fmt.Fprintf(&hex, "%02x ", c)
			if c >= 0x20 && c < 0x7f {
				ascii.WriteByte(c)
			} else {
				ascii.WriteByte('.')
			}
		}
		for i := len(chunk); i < per; i++ {
			hex.WriteString("   ")
		}
		lines = append(lines, hex.String()+" "+ascii.String())
	}
	return lines
}
