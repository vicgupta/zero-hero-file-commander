package main

import (
	"strconv"
	"strings"
)

// humanSize renders a byte count compactly, NC-style.
func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return trimF(float64(n)/(1<<30)) + "G"
	case n >= 1<<20:
		return trimF(float64(n)/(1<<20)) + "M"
	case n >= 1<<10:
		return trimF(float64(n)/(1<<10)) + "K"
	default:
		return strconv.FormatInt(n, 10)
	}
}

func trimF(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	return s
}

// naturalLess compares a and b case-insensitively, treating digit runs numerically.
func naturalLess(a, b string) bool {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		ca, cb := a[ia], b[ib]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if isDigit(ca) && isDigit(cb) {
			ja, jb := ia, ib
			for ja < len(a) && isDigit(a[ja]) {
				ja++
			}
			for jb < len(b) && isDigit(b[jb]) {
				jb++
			}
			na, _ := strconv.ParseUint(strings.TrimLeft(a[ia:ja], "0"), 10, 64)
			nb, _ := strconv.ParseUint(strings.TrimLeft(b[ib:jb], "0"), 10, 64)
			if na != nb {
				return na < nb
			}
			if ja-ia != jb-ib {
				return ja-ia < jb-ib
			}
			ia, ib = ja, jb
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ia++
		ib++
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// padRune pads/truncates s to width n, counting runes, not bytes.
func padRune(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

// padLeft right-aligns s within n cells, counting runes.
func padLeft(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[len(r)-n:])
	}
	return strings.Repeat(" ", n-len(r)) + s
}

// truncRune shortens s to at most n runes.
func truncRune(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// globMatch reports whether s matches pattern p with * and ? wildcards.
func globMatch(s, p string) bool {
	si, pi := 0, 0
	star, mark := -1, 0
	for si < len(s) {
		if pi < len(p) && (p[pi] == '?' || p[pi] == s[si]) {
			si++
			pi++
		} else if pi < len(p) && p[pi] == '*' {
			star = pi
			mark = si
			pi++
		} else if star != -1 {
			pi = star + 1
			mark++
			si = mark
		} else {
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// matchMask matches a filename against a ;-separated mask list (e.g. "*.go;*.md").
func matchMask(name, mask string) bool {
	lower := strings.ToLower(name)
	for _, part := range strings.Split(mask, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if globMatch(lower, strings.ToLower(part)) {
			return true
		}
	}
	return false
}
