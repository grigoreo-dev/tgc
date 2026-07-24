package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetFor(t *testing.T) {
	rel := &Release{Assets: []Asset{
		{Name: "tgc_1.3.0_linux_amd64.tar.gz"},
		{Name: "tgc_1.3.0_darwin_arm64.tar.gz"},
	}}
	a, err := assetFor(rel, "darwin", "arm64")
	if err != nil || a.Name != "tgc_1.3.0_darwin_arm64.tar.gz" {
		t.Fatalf("assetFor darwin/arm64 = %+v, %v", a, err)
	}
	if _, err := assetFor(rel, "windows", "386"); err == nil {
		t.Fatalf("assetFor windows/386 expected error")
	}
}

func makeTarGz(t *testing.T, binName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: binName, Mode: 0o755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	tw.Write(content)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestDownloadAndVerify(t *testing.T) {
	content := []byte("#!/bin/sh\necho hi\n")
	targz := makeTarGz(t, "tgc", content)
	sum := sha256.Sum256(targz)
	assetName := "tgc_1.3.0_darwin_arm64.tar.gz"
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

	mux := http.NewServeMux()
	mux.HandleFunc("/a.tgz", func(w http.ResponseWriter, r *http.Request) { w.Write(targz) })
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(checksums)) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	asset := &Asset{Name: assetName, URL: srv.URL + "/a.tgz"}
	bin, cleanup, err := downloadAndVerify(context.Background(), srv.Client(), asset, srv.URL+"/checksums.txt")
	if err != nil {
		t.Fatalf("downloadAndVerify err: %v", err)
	}
	defer cleanup()
	got, _ := os.ReadFile(bin)
	if !bytes.Equal(got, content) {
		t.Fatalf("extracted binary content mismatch")
	}
}

func TestReplaceRunningAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app")
	os.WriteFile(target, []byte("old"), 0o755)
	newBin := filepath.Join(t.TempDir(), "new")
	os.WriteFile(newBin, []byte("new"), 0o755)

	if err := replaceFileAtomic(newBin, target); err != nil {
		t.Fatalf("replaceFileAtomic err: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Fatalf("target content = %q, want new", got)
	}
}

func TestDownloadAndVerifyRejectsOversizedBinary(t *testing.T) {
	// Build a tar.gz whose "tgc" member exceeds the extraction cap.
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	big := maxExtractedBinary + 1024
	if err := tw.WriteHeader(&tar.Header{Name: "tgc", Mode: 0o755, Size: int64(big), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	// Stream zeros without allocating `big` bytes at once.
	if _, err := io.CopyN(tw, zeroReader{}, int64(big)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	data := raw.Bytes()

	sum := sha256.Sum256(data)
	checksums := []byte(hex.EncodeToString(sum[:]) + "  bomb_linux_amd64.tar.gz\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			_, _ = w.Write(checksums)
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	asset := &Asset{Name: "bomb_linux_amd64.tar.gz", URL: srv.URL + "/a.tgz"}
	_, cleanup, err := downloadAndVerify(context.Background(), srv.Client(), asset, srv.URL+"/checksums.txt")
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected archive error for oversized binary, got nil")
	}
	if !strings.Contains(err.Error(), "archive") {
		t.Fatalf("expected archive error, got %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
