package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"1.2.0", "v1.3.0", true},
		{"1.3.0", "v1.3.0", false},
		{"1.3.0", "1.2.0", false},
		{"dev", "v9.9.9", false},
	}
	for _, c := range cases {
		got, err := Newer(c.cur, c.latest)
		if err != nil {
			t.Fatalf("Newer(%q,%q) err: %v", c.cur, c.latest, err)
		}
		if got != c.want {
			t.Fatalf("Newer(%q,%q) = %v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer Tok" {
			t.Errorf("Authorization = %q, want Bearer Tok", got)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0","assets":[{"name":"tgc_1.3.0_darwin_arm64.tar.gz","browser_download_url":"http://x/a.tgz"}]}`))
	}))
	defer srv.Close()

	t.Setenv("GITHUB_TOKEN", "Tok")
	rel, err := latestReleaseFrom(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("latestReleaseFrom err: %v", err)
	}
	if rel.Tag != "v1.3.0" || len(rel.Assets) != 1 {
		t.Fatalf("parsed release = %+v", rel)
	}
	if rel.Assets[0].Name != "tgc_1.3.0_darwin_arm64.tar.gz" {
		t.Fatalf("asset name = %q", rel.Assets[0].Name)
	}
}

func TestLatestRelease404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := latestReleaseFrom(context.Background(), srv.Client(), srv.URL)
	if !errors.Is(err, ErrNoReleases) {
		t.Fatalf("err = %v, want ErrNoReleases", err)
	}
}
