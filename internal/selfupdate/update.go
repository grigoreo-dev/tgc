package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/grigoreo-dev/tgc/internal/output"
	"github.com/grigoreo-dev/tgc/internal/version"
)

// CheckResult reports the outcome of a self-update check/apply.
type CheckResult struct {
	Current              string   `json:"current"`
	Latest               string   `json:"latest"`
	UpdateAvailable      bool     `json:"update_available"`
	CompletionsRefreshed []string `json:"completions_refreshed,omitempty"`
}

// PostApplyFunc runs after a successful binary replace (e.g. refresh marked
// completion files). It must not import cli/cobra; the CLI layer supplies the
// implementation. Errors are non-fatal: Update still reports success.
type PostApplyFunc func() (refreshed []string, err error)

// Option configures Update.
type Option func(*updateConfig)

type updateConfig struct {
	postApply PostApplyFunc
}

// WithPostApply registers a hook invoked only after replaceRunning succeeds.
// Hook errors do not fail the update; refreshed paths (on success) are copied
// into CheckResult.CompletionsRefreshed.
func WithPostApply(fn PostApplyFunc) Option {
	return func(c *updateConfig) { c.postApply = fn }
}

// Test-injectable seams so Update's network/filesystem happy path can be
// exercised without real downloads or replacing the test binary.
var (
	latestReleaseFn     = LatestRelease
	downloadAndVerifyFn = downloadAndVerify
	replaceRunningFn    = replaceRunning
)

// defaultClient is for small, fast calls (release JSON, checksums.txt): a
// wall-clock timeout is a fine safety net there.
func defaultClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

// downloadClient is for the multi-MB binary download, where a global wall-clock
// timeout would abort a legitimate slow transfer. The request's context (set by
// the caller, e.g. `self update`'s 60s ctx) bounds it instead.
func downloadClient() *http.Client { return &http.Client{} }

// Check reports whether a newer release exists. No releases yet → not available.
func Check(ctx context.Context) (*CheckResult, error) {
	res := &CheckResult{Current: version.Version}
	rel, err := LatestRelease(ctx, defaultClient())
	if errors.Is(err, ErrNoReleases) {
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	newer, err := Newer(version.Version, rel.Tag)
	if err != nil {
		return nil, err
	}
	res.Latest = rel.Tag
	res.UpdateAvailable = newer
	return res, nil
}

func checksumsURL(rel *Release) string {
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			return a.URL
		}
	}
	return ""
}

// Update downloads and installs the latest release if newer than the running build.
// Optional WithPostApply runs only after a successful binary replace.
func Update(ctx context.Context, opts ...Option) (*CheckResult, error) {
	cfg := updateConfig{}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}

	if version.IsDev() {
		return nil, output.Errf("dev_build", "cannot self-update a dev build; install a release via install.sh")
	}
	res := &CheckResult{Current: version.Version}
	rel, err := latestReleaseFn(ctx, defaultClient())
	if errors.Is(err, ErrNoReleases) {
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	newer, err := Newer(version.Version, rel.Tag)
	if err != nil {
		return nil, err
	}
	res.Latest = rel.Tag
	res.UpdateAvailable = newer
	if !newer {
		return res, nil
	}
	asset, err := assetFor(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	cu := checksumsURL(rel)
	if cu == "" {
		return nil, output.Errf("checksum", "release %s has no checksums.txt", rel.Tag)
	}
	bin, cleanup, err := downloadAndVerifyFn(ctx, downloadClient(), asset, cu)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if err := replaceRunningFn(bin); err != nil {
		return nil, err
	}
	if cfg.postApply != nil {
		refreshed, applyErr := cfg.postApply()
		if applyErr != nil {
			// Non-fatal: binary is already replaced; surface via caller warn.
			// CompletionsRefreshed stays empty so JSON omits the field.
			_ = applyErr
		} else if len(refreshed) > 0 {
			res.CompletionsRefreshed = append([]string(nil), refreshed...)
		}
	}
	return res, nil
}
