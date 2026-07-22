package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"testing"

	"github.com/grigoreo-dev/tgc/internal/version"
)

// TestPostApplyHook_OnlyOnSuccessfulReplace proves the post-apply hook:
//   - runs after a successful replace path
//   - does not run on up-to-date / no-release paths
//   - errors from the hook do not fail Update
func TestPostApplyHook_OnlyOnSuccessfulReplace(t *testing.T) {
	oldVer := version.Version
	t.Cleanup(func() { version.Version = oldVer })
	version.Version = "1.0.0"

	// --- up-to-date: release tag equals current → no replace, no hook ---
	t.Run("not_called_when_up_to_date", func(t *testing.T) {
		restore := installUpdateSeams(t, updateSeams{
			latest: func(ctx context.Context, c *http.Client) (*Release, error) {
				return &Release{Tag: "v1.0.0"}, nil
			},
		})
		defer restore()

		var calls int
		res, err := Update(context.Background(), WithPostApply(func() ([]string, error) {
			calls++
			return []string{"/tmp/x"}, nil
		}))
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if res.UpdateAvailable {
			t.Fatalf("UpdateAvailable = true, want false")
		}
		if calls != 0 {
			t.Fatalf("post-apply called %d times on up-to-date path, want 0", calls)
		}
		if len(res.CompletionsRefreshed) != 0 {
			t.Fatalf("CompletionsRefreshed = %v, want empty", res.CompletionsRefreshed)
		}
	})

	// --- no releases: ErrNoReleases → no replace, no hook ---
	t.Run("not_called_when_no_releases", func(t *testing.T) {
		restore := installUpdateSeams(t, updateSeams{
			latest: func(ctx context.Context, c *http.Client) (*Release, error) {
				return nil, ErrNoReleases
			},
		})
		defer restore()

		var calls int
		res, err := Update(context.Background(), WithPostApply(func() ([]string, error) {
			calls++
			return nil, nil
		}))
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if res.UpdateAvailable {
			t.Fatalf("UpdateAvailable = true, want false")
		}
		if calls != 0 {
			t.Fatalf("post-apply called %d times on no-releases path, want 0", calls)
		}
	})

	// --- successful replace: hook runs, paths recorded ---
	t.Run("called_after_successful_replace", func(t *testing.T) {
		restore := installUpdateSeams(t, updateSeams{
			latest: func(ctx context.Context, c *http.Client) (*Release, error) {
				return newerReleaseWithAsset(), nil
			},
			download: func(ctx context.Context, c *http.Client, asset *Asset, checksumsURL string) (string, func(), error) {
				return "/tmp/fake-bin", func() {}, nil
			},
			replace: func(newBin string) error {
				if newBin != "/tmp/fake-bin" {
					t.Fatalf("replaceRunning got %q", newBin)
				}
				return nil
			},
		})
		defer restore()

		var calls int
		wantPaths := []string{"/home/u/.local/share/bash-completion/completions/tgc"}
		res, err := Update(context.Background(), WithPostApply(func() ([]string, error) {
			calls++
			return wantPaths, nil
		}))
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if !res.UpdateAvailable {
			t.Fatalf("UpdateAvailable = false, want true")
		}
		if calls != 1 {
			t.Fatalf("post-apply called %d times, want 1", calls)
		}
		if len(res.CompletionsRefreshed) != 1 || res.CompletionsRefreshed[0] != wantPaths[0] {
			t.Fatalf("CompletionsRefreshed = %v, want %v", res.CompletionsRefreshed, wantPaths)
		}
	})

	// --- hook error: Update still succeeds; CompletionsRefreshed empty ---
	t.Run("hook_error_does_not_fail_update", func(t *testing.T) {
		restore := installUpdateSeams(t, updateSeams{
			latest: func(ctx context.Context, c *http.Client) (*Release, error) {
				return newerReleaseWithAsset(), nil
			},
			download: func(ctx context.Context, c *http.Client, asset *Asset, checksumsURL string) (string, func(), error) {
				return "/tmp/fake-bin", func() {}, nil
			},
			replace: func(string) error { return nil },
		})
		defer restore()

		var calls int
		res, err := Update(context.Background(), WithPostApply(func() ([]string, error) {
			calls++
			return []string{"/should/not/appear"}, errors.New("refresh boom")
		}))
		if err != nil {
			t.Fatalf("Update should swallow hook error, got: %v", err)
		}
		if !res.UpdateAvailable {
			t.Fatalf("UpdateAvailable = false, want true (replace succeeded)")
		}
		if calls != 1 {
			t.Fatalf("post-apply called %d times, want 1", calls)
		}
		if len(res.CompletionsRefreshed) != 0 {
			t.Fatalf("CompletionsRefreshed = %v on hook error, want empty", res.CompletionsRefreshed)
		}
	})

	// --- replace failure: hook must not run ---
	t.Run("not_called_when_replace_fails", func(t *testing.T) {
		restore := installUpdateSeams(t, updateSeams{
			latest: func(ctx context.Context, c *http.Client) (*Release, error) {
				return newerReleaseWithAsset(), nil
			},
			download: func(ctx context.Context, c *http.Client, asset *Asset, checksumsURL string) (string, func(), error) {
				return "/tmp/fake-bin", func() {}, nil
			},
			replace: func(string) error { return errors.New("permission denied") },
		})
		defer restore()

		var calls int
		_, err := Update(context.Background(), WithPostApply(func() ([]string, error) {
			calls++
			return nil, nil
		}))
		if err == nil {
			t.Fatalf("Update: want replace error, got nil")
		}
		if calls != 0 {
			t.Fatalf("post-apply called %d times after failed replace, want 0", calls)
		}
	})
}

type updateSeams struct {
	latest   func(context.Context, *http.Client) (*Release, error)
	download func(context.Context, *http.Client, *Asset, string) (string, func(), error)
	replace  func(string) error
}

// installUpdateSeams swaps package-level Update seams and returns a restore func.
func installUpdateSeams(t *testing.T, s updateSeams) func() {
	t.Helper()
	oldLatest := latestReleaseFn
	oldDownload := downloadAndVerifyFn
	oldReplace := replaceRunningFn
	if s.latest != nil {
		latestReleaseFn = s.latest
	}
	if s.download != nil {
		downloadAndVerifyFn = s.download
	}
	if s.replace != nil {
		replaceRunningFn = s.replace
	}
	return func() {
		latestReleaseFn = oldLatest
		downloadAndVerifyFn = oldDownload
		replaceRunningFn = oldReplace
	}
}

func newerReleaseWithAsset() *Release {
	name := "tgc_9.9.9_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	return &Release{
		Tag: "v9.9.9",
		Assets: []Asset{
			{Name: name, URL: "http://example.invalid/a.tgz"},
			{Name: "checksums.txt", URL: "http://example.invalid/checksums.txt"},
		},
	}
}
