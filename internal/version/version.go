// Package version exposes the tgc build version. Version is overwritten at
// release time via -ldflags "-X .../internal/version.Version=<semver>".
package version

// Version is the build version. "dev" means an unstamped local build.
var Version = "dev"

// IsDev reports whether this is an unstamped development build.
func IsDev() bool { return Version == "dev" }
