package version

import "testing"

func TestIsDev(t *testing.T) {
	old := Version
	defer func() { Version = old }()

	Version = "dev"
	if !IsDev() {
		t.Fatalf("IsDev() = false for dev, want true")
	}
	Version = "1.2.3"
	if IsDev() {
		t.Fatalf("IsDev() = true for 1.2.3, want false")
	}
}
