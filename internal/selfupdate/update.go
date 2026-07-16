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

type CheckResult struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
}

func defaultClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

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
func Update(ctx context.Context) (*CheckResult, error) {
	if version.IsDev() {
		return nil, output.Errf("dev_build", "cannot self-update a dev build; install a release via install.sh")
	}
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
	bin, cleanup, err := downloadAndVerify(ctx, defaultClient(), asset, cu)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if err := replaceRunning(bin); err != nil {
		return nil, err
	}
	return res, nil
}
