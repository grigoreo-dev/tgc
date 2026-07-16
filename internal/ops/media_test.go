package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	const fallback = "file_7"
	cases := []struct {
		raw  string
		want string
	}{
		{"report.pdf", "report.pdf"},              // ordinary name kept
		{"../../etc/passwd", "passwd"},            // strip traversal, keep base
		{"/abs/path/x", "x"},                      // strip absolute prefix
		{"..", fallback},                          // dot-dot alone → fallback
		{".", fallback},                           // dot alone → fallback
		{"", fallback},                            // empty → fallback
		{"../../../../home/user/.ssh/authorized_keys", "authorized_keys"},
	}
	for _, c := range cases {
		got := sanitizeName(c.raw, fallback)
		if got != c.want {
			t.Errorf("sanitizeName(%q, %q) = %q, want %q", c.raw, fallback, got, c.want)
		}
		if strings.ContainsRune(got, '/') || strings.ContainsRune(got, os.PathSeparator) {
			t.Errorf("sanitizeName(%q) = %q contains a path separator", c.raw, got)
		}
		if got == ".." {
			t.Errorf("sanitizeName(%q) = %q is a traversal segment", c.raw, got)
		}
	}
}

func TestClassifyUpload(t *testing.T) {
	cases := []struct {
		path       string
		asDocument bool
		kind       string
	}{
		{"pic.jpg", false, "photo"},   // images are photos by default
		{"pic.jpg", true, "document"}, // --as-document forces document
		{"clip.mp4", false, "video"},
		{"song.mp3", false, "audio"},
		{"report.pdf", false, "document"},
		{"data.bin", true, "document"}, // as-document has no effect on non-images
	}
	for _, c := range cases {
		kind, _ := classifyUpload(c.path, c.asDocument)
		if kind != c.kind {
			t.Errorf("classifyUpload(%q, %v) = %q, want %q", c.path, c.asDocument, kind, c.kind)
		}
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if uniquePath(p) != p {
		t.Fatal("free path must be returned as-is")
	}
	_ = os.WriteFile(p, []byte("x"), 0o600)
	got := uniquePath(p)
	want := filepath.Join(dir, "file (1).txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSendFilesTooMany(t *testing.T) {
	files := make([]string, 11)
	for i := range files {
		files[i] = "f.jpg"
	}
	_, err := SendFiles(nil, "x", files, FileOpts{})
	if err == nil {
		t.Fatal("11 files must be rejected")
	}
}
