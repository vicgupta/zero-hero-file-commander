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
	path  string
	data  []byte
	lines []string
	hex   bool
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
	v := &viewer{path: path, data: buf}
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

func (v *viewer) render() {
	if v.hex {
		v.lines = hexDump(v.data)
	} else {
		v.lines = textLines(v.data)
	}
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
