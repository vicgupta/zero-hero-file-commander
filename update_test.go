package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAssetSuffix(t *testing.T) {
	cases := []struct {
		goos, goarch, wantSuffix, wantBin string
	}{
		{"darwin", "amd64", "_darwin_amd64.tar.gz", "zhfc"},
		{"darwin", "arm64", "_darwin_arm64.tar.gz", "zhfc"},
		{"linux", "amd64", "_linux_amd64.tar.gz", "zhfc"},
		{"linux", "arm64", "_linux_arm64.tar.gz", "zhfc"},
		{"windows", "amd64", "_windows_amd64.zip", "zhfc.exe"},
	}
	for _, c := range cases {
		suffix, bin := assetSuffix(c.goos, c.goarch)
		if suffix != c.wantSuffix || bin != c.wantBin {
			t.Errorf("assetSuffix(%s,%s) = (%q,%q), want (%q,%q)", c.goos, c.goarch, suffix, bin, c.wantSuffix, c.wantBin)
		}
	}
}

func TestPickAsset(t *testing.T) {
	rel := ghRelease{TagName: "v1.2.3", Assets: []ghAsset{
		{Name: "zhfc_v1.2.3_linux_amd64.tar.gz"},
		{Name: "zhfc_v1.2.3_darwin_arm64.tar.gz"},
		{Name: "SHA256SUMS.txt"},
	}}
	a, err := pickAsset(rel, "_darwin_arm64.tar.gz")
	if err != nil || a.Name != "zhfc_v1.2.3_darwin_arm64.tar.gz" {
		t.Fatalf("pickAsset = %+v, %v", a, err)
	}
	if _, err := pickAsset(rel, "_windows_amd64.zip"); err == nil {
		t.Error("expected an error when no asset matches the suffix")
	}
}

func TestPickChecksums(t *testing.T) {
	rel := ghRelease{Assets: []ghAsset{{Name: "zhfc_v1.2.3_linux_amd64.tar.gz"}, {Name: "SHA256SUMS.txt"}}}
	a, err := pickChecksums(rel)
	if err != nil || a.Name != "SHA256SUMS.txt" {
		t.Fatalf("pickChecksums = %+v, %v", a, err)
	}
	if _, err := pickChecksums(ghRelease{}); err == nil {
		t.Error("expected an error when SHA256SUMS.txt is missing")
	}
}

func sha256sumsLine(name string, data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name)
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("archive contents")
	sums := []byte(sha256sumsLine("other.tar.gz", []byte("unrelated")) + sha256sumsLine("archive.tar.gz", data))

	if err := verifyChecksum(sums, "archive.tar.gz", data); err != nil {
		t.Errorf("verifyChecksum with a matching entry: %v", err)
	}
	if err := verifyChecksum(sums, "archive.tar.gz", []byte("tampered")); err == nil {
		t.Error("verifyChecksum should fail when the data doesn't match the recorded hash")
	}
	if err := verifyChecksum(sums, "missing.tar.gz", data); err == nil {
		t.Error("verifyChecksum should fail when there's no entry for the filename")
	}
}

func buildTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0o755, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func buildZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	// Mirrors the real release layout: a top-level versioned directory
	// containing the binary alongside docs.
	archive := buildTarGz(t, map[string][]byte{
		"zhfc_v1.2.3_linux_amd64/zhfc":      []byte("binary content"),
		"zhfc_v1.2.3_linux_amd64/README.md": []byte("docs"),
	})
	got, err := extractBinary("zhfc_v1.2.3_linux_amd64.tar.gz", archive, "zhfc")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if string(got) != "binary content" {
		t.Errorf("extracted = %q, want %q", got, "binary content")
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	archive := buildZip(t, map[string][]byte{
		"zhfc_v1.2.3_windows_amd64/zhfc.exe":  []byte("windows binary"),
		"zhfc_v1.2.3_windows_amd64/README.md": []byte("docs"),
	})
	got, err := extractBinary("zhfc_v1.2.3_windows_amd64.zip", archive, "zhfc.exe")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if string(got) != "windows binary" {
		t.Errorf("extracted = %q, want %q", got, "windows binary")
	}
}

func TestExtractBinaryMissingFile(t *testing.T) {
	archive := buildTarGz(t, map[string][]byte{"dir/other-file": []byte("x")})
	if _, err := extractBinary("x.tar.gz", archive, "zhfc"); err == nil {
		t.Error("expected an error when the binary isn't present in the archive")
	}
}

func TestExtractBinaryUnsupportedFormat(t *testing.T) {
	if _, err := extractBinary("archive.rar", []byte("x"), "zhfc"); err == nil {
		t.Error("expected an error for an unrecognized archive extension")
	}
}

func TestReplaceExecutableInstallsNewContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "zhfc")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(target, []byte("new binary"), 0o755); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("target content = %q, want %q", got, "new binary")
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o755 {
			t.Errorf("mode = %v, want 0755", st.Mode().Perm())
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries after update, want exactly 1 (no .old or temp files left behind): %v", len(entries), entries)
	}
}

func TestReplaceExecutableWithNoExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "zhfc")
	if err := replaceExecutable(target, []byte("fresh install"), 0o755); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fresh install" {
		t.Errorf("content = %q, want %q", got, "fresh install")
	}
}

// The whole point of moving the original aside before writing the
// replacement: if the write step fails, the original binary must still be
// usable afterward, not left half-replaced or missing.
func TestReplaceExecutableRollsBackOnWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission semantics differ on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "zhfc")
	if err := os.WriteFile(target, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Make the directory read-only so creating the temp file for the new
	// binary fails after the original has already been moved aside.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755) // restore so t.TempDir() cleanup can remove it

	err := replaceExecutable(target, []byte("new"), 0o755)
	if err == nil {
		t.Fatal("expected an error when the directory isn't writable")
	}
	os.Chmod(dir, 0o755)
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("original binary missing after a failed update: %v", readErr)
	}
	if string(got) != "original" {
		t.Errorf("original binary content = %q, want %q (rollback should restore it)", got, "original")
	}
}

// ghTestServer builds a fake GitHub API + asset host serving one release.
func ghTestServer(t *testing.T, tag string, archive []byte, archiveName string, sums []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/"+updateRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rel := ghRelease{TagName: tag, Assets: []ghAsset{
			{Name: archiveName, BrowserDownloadURL: srv.URL + "/asset"},
			{Name: "SHA256SUMS.txt", BrowserDownloadURL: srv.URL + "/sums"},
		}}
		json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) { w.Write(sums) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func currentPlatformArchive(t *testing.T, binContent []byte) (archiveName string, archive []byte) {
	t.Helper()
	suffix, binName := assetSuffix(runtime.GOOS, runtime.GOARCH)
	archiveName = "zhfc_v9.9.9" + suffix
	dir := strings.TrimSuffix(strings.TrimSuffix(archiveName, ".tar.gz"), ".zip")
	files := map[string][]byte{dir + "/" + binName: binContent}
	if runtime.GOOS == "windows" {
		archive = buildZip(t, files)
	} else {
		archive = buildTarGz(t, files)
	}
	return archiveName, archive
}

func TestDoUpdateEndToEnd(t *testing.T) {
	archiveName, archive := currentPlatformArchive(t, []byte("new binary v9.9.9"))
	sums := []byte(sha256sumsLine(archiveName, archive))
	srv := ghTestServer(t, "v9.9.9", archive, archiveName, sums)

	dir := t.TempDir()
	target := filepath.Join(dir, "zhfc")
	if err := os.WriteFile(target, []byte("old binary v0.1.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := doUpdate(&out, http.DefaultClient, srv.URL, updateRepo, "0.1.0", target)
	if err != nil {
		t.Fatalf("doUpdate: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new binary v9.9.9" {
		t.Errorf("target content = %q, want %q", got, "new binary v9.9.9")
	}
	if !strings.Contains(out.String(), "v0.1.0") || !strings.Contains(out.String(), "v9.9.9") {
		t.Errorf("output should mention both versions: %q", out.String())
	}
}

func TestDoUpdateAlreadyUpToDate(t *testing.T) {
	hitAssetEndpoint := false
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/"+updateRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ghRelease{TagName: "v0.1.0"})
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) { hitAssetEndpoint = true })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	target := filepath.Join(dir, "zhfc")
	os.WriteFile(target, []byte("unchanged"), 0o755)

	var out bytes.Buffer
	if err := doUpdate(&out, http.DefaultClient, srv.URL, updateRepo, "0.1.0", target); err != nil {
		t.Fatalf("doUpdate: %v", err)
	}
	if !strings.Contains(out.String(), "Already up to date") {
		t.Errorf("output = %q, want an up-to-date message", out.String())
	}
	if hitAssetEndpoint {
		t.Error("already up to date, but doUpdate still tried to download an asset")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "unchanged" {
		t.Error("target binary was modified despite already being up to date")
	}
}

func TestDoUpdateChecksumMismatchLeavesBinaryUntouched(t *testing.T) {
	archiveName, archive := currentPlatformArchive(t, []byte("new binary"))
	wrongSums := []byte(sha256sumsLine(archiveName, []byte("not the real archive")))
	srv := ghTestServer(t, "v9.9.9", archive, archiveName, wrongSums)

	dir := t.TempDir()
	target := filepath.Join(dir, "zhfc")
	os.WriteFile(target, []byte("original"), 0o755)

	err := doUpdate(&bytes.Buffer{}, http.DefaultClient, srv.URL, updateRepo, "0.1.0", target)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want a checksum mismatch error", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "original" {
		t.Error("binary was replaced despite a checksum mismatch")
	}
}

func TestDoUpdateNoBuildForPlatform(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+updateRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ghRelease{TagName: "v9.9.9", Assets: []ghAsset{{Name: "zhfc_v9.9.9_plan9_amd64.tar.gz"}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := doUpdate(&bytes.Buffer{}, http.DefaultClient, srv.URL, updateRepo, "0.1.0", filepath.Join(t.TempDir(), "zhfc"))
	if err == nil {
		t.Fatal("expected an error when no release asset matches this platform")
	}
}
