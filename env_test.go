package main

import (
	"os"
	"strings"
	"testing"
)

// TestSeedPATH_AddsHomebrewLocations verifies the PATH seeder is a
// net-add — it should insert Homebrew and MacTeX prefixes even when
// they aren't in the starting PATH. This is what makes launched-from-
// Finder builds see pandoc, soffice, ffmpeg, etc.
//
// The user regression was: PATH="/usr/bin:/bin:/usr/sbin:/sbin" ->
// pandoc-text probes as unavailable -> docx conversions fail with
// "no plugin can extract text from docx".
func TestSeedPATH_AddsHomebrewLocations(t *testing.T) {
	origPATH := os.Getenv("PATH")
	origSHELL := os.Getenv("SHELL")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPATH)
		_ = os.Setenv("SHELL", origSHELL)
	})

	// Simulate the Finder-launched minimal environment.
	_ = os.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	// Empty SHELL so the login-shell branch is skipped (avoids depending
	// on the developer's dotfiles for test outcomes).
	_ = os.Setenv("SHELL", "")

	seedPATH()

	after := os.Getenv("PATH")
	parts := strings.Split(after, ":")

	// At least one of the two Homebrew prefixes must be present.
	hasBrew := false
	for _, p := range parts {
		if p == "/opt/homebrew/bin" || p == "/usr/local/bin" {
			hasBrew = true
			break
		}
	}
	if !hasBrew {
		t.Errorf("seedPATH did not add either /opt/homebrew/bin or /usr/local/bin. PATH=%s", after)
	}

	// The original entries must still be reachable.
	for _, must := range []string{"/usr/bin", "/bin"} {
		found := false
		for _, p := range parts {
			if p == must {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("seedPATH dropped %s from PATH: %s", must, after)
		}
	}
}

// TestSeedPATH_Idempotent verifies calling seedPATH twice doesn't
// duplicate entries (safe to run on hot-reload / re-init paths).
func TestSeedPATH_Idempotent(t *testing.T) {
	origPATH := os.Getenv("PATH")
	origSHELL := os.Getenv("SHELL")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", origPATH)
		_ = os.Setenv("SHELL", origSHELL)
	})
	_ = os.Setenv("PATH", "/usr/bin:/bin")
	_ = os.Setenv("SHELL", "")

	seedPATH()
	first := os.Getenv("PATH")
	seedPATH()
	second := os.Getenv("PATH")

	if first != second {
		t.Errorf("seedPATH is not idempotent:\n  first:  %s\n  second: %s", first, second)
	}

	// Also sanity: no entry appears twice
	seen := map[string]bool{}
	for _, p := range strings.Split(second, ":") {
		if seen[p] && p != "" {
			t.Errorf("PATH has duplicate entry %q: %s", p, second)
		}
		seen[p] = true
	}
}
