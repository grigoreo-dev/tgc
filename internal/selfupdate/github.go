// Package selfupdate implements tgc's binary self-update over GitHub releases.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/grigoreo-dev/tgc/internal/output"
	"golang.org/x/mod/semver"
)

const releaseAPI = "https://api.github.com/repos/grigoreo-dev/tgc/releases/latest"

// ErrNoReleases means the repo has no published releases yet (HTTP 404).
var ErrNoReleases = errors.New("no releases")

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}

// LatestRelease fetches the latest release from the tgc repo.
func LatestRelease(ctx context.Context, c *http.Client) (*Release, error) {
	return latestReleaseFrom(ctx, c, releaseAPI)
}

func latestReleaseFrom(ctx context.Context, c *http.Client, url string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := githubToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, output.Errf("network", "github request failed: %v", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNoReleases
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, output.Errf("rate_limited", "GitHub API rate limit; set GITHUB_TOKEN or retry later")
	default:
		return nil, output.Errf("github", "unexpected status %d from GitHub", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, output.Errf("github", "cannot parse release JSON: %v", err)
	}
	return &rel, nil
}

// Newer reports whether latestTag is a newer SemVer than current.
// A "dev" current is never considered older (returns false).
func Newer(current, latestTag string) (bool, error) {
	if current == "dev" {
		return false, nil
	}
	cur := ensureV(current)
	lat := ensureV(latestTag)
	if !semver.IsValid(cur) {
		return false, output.Errf("bad_version", "current version %q is not valid semver", current)
	}
	if !semver.IsValid(lat) {
		return false, output.Errf("bad_version", "release tag %q is not valid semver", latestTag)
	}
	return semver.Compare(lat, cur) > 0, nil
}

func ensureV(s string) string {
	if len(s) > 0 && s[0] == 'v' {
		return s
	}
	return "v" + s
}
