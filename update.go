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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// updateRepo is the GitHub repo self-update downloads releases from.
const updateRepo = "vicgupta/zero-hero-file-commander"

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// fetchLatestRelease reads the repo's newest release metadata from the
// GitHub API. apiBase is a parameter (not a constant) so tests can point it
// at an httptest.Server instead of the real api.github.com.
func fetchLatestRelease(client *http.Client, apiBase, repo string) (ghRelease, error) {
	url := apiBase + "/repos/" + repo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ghRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "zhfc-updater")
	resp, err := client.Do(req)
	if err != nil {
		return ghRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ghRelease{}, fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return ghRelease{}, fmt.Errorf("decoding release metadata: %w", err)
	}
	return rel, nil
}

// assetSuffix returns the release-archive filename suffix and the binary's
// name inside that archive for a given platform, matching the naming this
// project's own release process produces (zhfc_<version>_<goos>_<goarch>.*).
func assetSuffix(goos, goarch string) (suffix, binName string) {
	if goos == "windows" {
		return fmt.Sprintf("_%s_%s.zip", goos, goarch), "zhfc.exe"
	}
	return fmt.Sprintf("_%s_%s.tar.gz", goos, goarch), "zhfc"
}

func pickAsset(rel ghRelease, suffix string) (ghAsset, error) {
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, suffix) {
			return a, nil
		}
	}
	return ghAsset{}, fmt.Errorf("release %s has no build ending in %q", rel.TagName, suffix)
}

func pickChecksums(rel ghRelease) (ghAsset, error) {
	for _, a := range rel.Assets {
		if a.Name == "SHA256SUMS.txt" {
			return a, nil
		}
	}
	return ghAsset{}, fmt.Errorf("release %s has no SHA256SUMS.txt", rel.TagName)
}

func downloadBytes(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "zhfc-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// verifyChecksum checks data's SHA-256 against its entry in a
// SHA256SUMS.txt-formatted file (as produced by `shasum -a 256`, one
// "<hex-hash>  <filename>" line per file).
func verifyChecksum(sums []byte, filename string, data []byte) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-1] == filename {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s", filename)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filename, got, want)
	}
	return nil
}

// extractBinary pulls the file named binName out of a .tar.gz or .zip archive.
func extractBinary(archiveName string, data []byte, binName string) ([]byte, error) {
	switch {
	case strings.HasSuffix(archiveName, ".tar.gz"):
		return extractFromTarGz(data, binName)
	case strings.HasSuffix(archiveName, ".zip"):
		return extractFromZip(data, binName)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", archiveName)
	}
}

func extractFromTarGz(data []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == binName {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binName)
}

func extractFromZip(data []byte, binName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == binName {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binName)
}

// replaceExecutable atomically swaps targetPath's contents for newData. It
// moves the current file aside to targetPath+".old" first, then installs the
// new content via a same-directory temp file and rename (so the rename is
// atomic — never a same-filesystem cross-device hop) and rolls the backup
// back into place if any step after that fails.
//
// This is safe to do to a binary that is currently executing: on Unix,
// renaming a file's directory entry away doesn't invalidate a process
// already running from the open file underneath it. On Windows, the loader
// maps executables with FILE_SHARE_DELETE, which permits exactly this
// rename-then-replace sequence even though the file can't be overwritten or
// deleted outright.
func replaceExecutable(targetPath string, newData []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(targetPath)
	backup := targetPath + ".old"
	os.Remove(backup) // best-effort: clear out a leftover from a prior update

	hadOriginal := false
	if _, statErr := os.Stat(targetPath); statErr == nil {
		if err := os.Rename(targetPath, backup); err != nil {
			return fmt.Errorf("could not move the current binary aside: %w", err)
		}
		hadOriginal = true
	}
	restore := func() {
		if hadOriginal {
			os.Rename(backup, targetPath)
		}
	}

	tmp, err := os.CreateTemp(dir, ".zhfc-update-*")
	if err != nil {
		restore()
		return fmt.Errorf("could not create a temp file next to %s: %w", targetPath, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
			restore()
		}
	}()

	if _, err = tmp.Write(newData); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write the new binary: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("could not install the new binary: %w", err)
	}
	os.Remove(backup) // best-effort; harmless if a moment-locked old copy lingers
	return nil
}

// doUpdate is runUpdate's testable core: it takes the executable path to
// replace explicitly rather than discovering it via os.Executable(), so
// tests can point it at a scratch file instead of the running test binary.
func doUpdate(out io.Writer, client *http.Client, apiBase, repo, currentVersion, execPath string) error {
	fmt.Fprintln(out, "Checking for updates...")
	rel, err := fetchLatestRelease(client, apiBase, repo)
	if err != nil {
		return fmt.Errorf("checking latest release: %w", err)
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	if latest == currentVersion {
		fmt.Fprintf(out, "Already up to date (v%s).\n", currentVersion)
		return nil
	}

	suffix, binName := assetSuffix(runtime.GOOS, runtime.GOARCH)
	asset, err := pickAsset(rel, suffix)
	if err != nil {
		return fmt.Errorf("finding a build for %s/%s: %w", runtime.GOOS, runtime.GOARCH, err)
	}
	sumsAsset, err := pickChecksums(rel)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Downloading %s (v%s -> v%s)...\n", asset.Name, currentVersion, latest)
	archiveData, err := downloadBytes(client, asset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", asset.Name, err)
	}
	sumsData, err := downloadBytes(client, sumsAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	if err := verifyChecksum(sumsData, asset.Name, archiveData); err != nil {
		return err
	}

	binData, err := extractBinary(asset.Name, archiveData, binName)
	if err != nil {
		return err
	}

	if err := replaceExecutable(execPath, binData, 0o755); err != nil {
		return err
	}
	fmt.Fprintf(out, "Updated %s: v%s -> v%s\n", execPath, currentVersion, latest)
	return nil
}

// runUpdate is the production entry point: it resolves the currently running
// executable's real path (following a symlink if it was launched through
// one, so the binary at the symlink's target gets replaced) and downloads
// and installs the latest release over it.
func runUpdate(out io.Writer, currentVersion string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating the running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return doUpdate(out, client, "https://api.github.com", updateRepo, currentVersion, execPath)
}
