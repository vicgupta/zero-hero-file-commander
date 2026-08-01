package main

import (
	"errors"
	"io/fs"
	"runtime"
	"strings"
	"testing"
)

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{512, "512"},
		{2048, "2K"},
		{1536, "1.5K"},
		{1 << 20, "1M"},
		{1 << 30, "1G"},
		{3 * (1 << 30), "3G"},
	}
	for _, c := range cases {
		if got := humanSize(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"a", "b", true},
		{"a", "B", true}, // case-insensitive: 'a' < 'b'
		{"B", "a", false},
		{"a1", "a2", true},
		{"a10", "a9", false}, // numeric chunk
		{"file2.txt", "file10.txt", true},
		{"a", "a", false},
		{"aa", "a", false},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		s, p string
		want bool
	}{
		{"foo.go", "*.go", true},
		{"foo.txt", "*.go", false},
		{"a.txt", "?.txt", true},
		{"ab.txt", "?.txt", false},
		{"x.log", "*", true},
		{"foo", "foo", true},
	}
	for _, c := range cases {
		if got := globMatch(c.s, c.p); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.s, c.p, got, c.want)
		}
	}
}

func TestMatchMask(t *testing.T) {
	if !matchMask("main.go", "*.go;*.md") {
		t.Error("multi-mask should match *.go")
	}
	if matchMask("main.java", "*.go;*.md") {
		t.Error("multi-mask should reject *.java")
	}
}

func TestTruncRune(t *testing.T) {
	got := truncRune("abcdef", 3)
	if got != "ab…" {
		t.Errorf("truncRune = %q, want %q", got, "ab…")
	}
}

func TestDescribeErrAddsAHintForPermissionErrorsOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("hint text is macOS-specific")
	}
	err := &fs.PathError{Op: "mkdir", Path: "/Volumes/x", Err: fs.ErrPermission}
	got := describeErr(err)
	if !strings.Contains(got, err.Error()) {
		t.Errorf("describeErr should preserve the original message: %q", got)
	}
	if !strings.Contains(got, "System Settings") {
		t.Errorf("describeErr should append the macOS permission hint: %q", got)
	}
}

func TestDescribeErrLeavesNonPermissionErrorsAlone(t *testing.T) {
	err := errors.New("some other failure")
	if got := describeErr(err); got != "some other failure" {
		t.Errorf("describeErr(%v) = %q, want the message unchanged", err, got)
	}
}
