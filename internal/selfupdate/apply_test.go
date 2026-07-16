package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
