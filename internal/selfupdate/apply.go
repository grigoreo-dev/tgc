package selfupdate

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/grigoreo-dev/tgc/internal/output"
)

// maxExtractedBinary bounds the extracted tgc binary to defend against a
// decompression bomb: a small .tar.gz that expands to an enormous file.
const maxExtractedBinary = 500 << 20 // 500 MiB

func assetFor(rel *Release, goos, goarch string) (*Asset, error) {
	want := fmt.Sprintf("%s_%s", goos, goarch)
	for i := range rel.Assets {
		n := rel.Assets[i].Name
		if strings.Contains(n, want) && strings.HasSuffix(n, ".tar.gz") {
			return &rel.Assets[i], nil
		}
	}
	return nil, output.Errf("no_asset", "no release asset for %s/%s", goos, goarch)
}

func httpGet(ctx context.Context, c *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if t := githubToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, output.Errf("network", "download failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, output.Errf("download", "unexpected status %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

// expectedSum finds the sha256 hex for assetName in a checksums.txt body.
func expectedSum(checksums []byte, assetName string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(string(checksums)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", output.Errf("checksum", "no checksum entry for %s", assetName)
}

// downloadAndVerify downloads asset + checksums, verifies sha256, extracts the
// tgc binary into a temp dir, and returns its path plus a cleanup func.
func downloadAndVerify(ctx context.Context, c *http.Client, asset *Asset, checksumsURL string) (string, func(), error) {
	data, err := httpGet(ctx, c, asset.URL)
	if err != nil {
		return "", nil, err
	}
	checksums, err := httpGet(ctx, c, checksumsURL)
	if err != nil {
		return "", nil, err
	}
	want, err := expectedSum(checksums, asset.Name)
	if err != nil {
		return "", nil, err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return "", nil, output.Errf("checksum", "sha256 mismatch for %s", asset.Name)
	}

	tmpDir, err := os.MkdirTemp("", "tgc-update-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		cleanup()
		return "", nil, output.Errf("archive", "gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	binPath := filepath.Join(tmpDir, "tgc")
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			return "", nil, output.Errf("archive", "tar: %v", err)
		}
		// filepath.Base strips any leading path (incl. "../"), so this is our
		// Zip-Slip guard: we ONLY ever write to tmpDir/tgc, never a traversed
		// path. Do NOT "simplify" this to filepath.Join(tmpDir, hdr.Name).
		if filepath.Base(hdr.Name) == "tgc" && hdr.Typeflag == tar.TypeReg {
			f, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				cleanup()
				return "", nil, err
			}
			n, err := io.Copy(f, io.LimitReader(tr, maxExtractedBinary+1))
			if err != nil {
				f.Close()
				cleanup()
				return "", nil, err
			}
			if n > maxExtractedBinary {
				f.Close()
				cleanup()
				return "", nil, output.Errf("archive", "extracted binary exceeds %d bytes", maxExtractedBinary)
			}
			f.Close()
			found = true
			break
		}
	}
	if !found {
		cleanup()
		return "", nil, output.Errf("archive", "tgc binary not found in %s", asset.Name)
	}
	return binPath, cleanup, nil
}

// replaceFileAtomic copies newBin into a temp file in the SAME directory as
// target (avoiding EXDEV), chmods 0755, then renames over target.
func replaceFileAtomic(newBin, target string) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".tgc-new-")
	if err != nil {
		if os.IsPermission(err) {
			return output.Errf("permission_denied", "cannot write to %s; re-run install.sh or use sudo", dir)
		}
		return err
	}
	tmpName := tmp.Name()
	src, err := os.Open(newBin)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	defer src.Close()
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()
	if err := os.Chmod(tmpName, 0o755); err != nil { //nosec G302 -- the CLI binary must be executable; 0600 would make it non-runnable
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return output.Errf("permission_denied", "cannot replace %s; re-run install.sh or use sudo", target)
	}
	return nil
}

// replaceRunning resolves the running binary (through symlinks) and replaces it.
func replaceRunning(newBin string) error {
	exe, err := os.Executable()
	if err != nil {
		return output.Errf("internal", "cannot locate running binary: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return replaceFileAtomic(newBin, exe)
}
