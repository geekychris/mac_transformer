package main

import (
	"os"
	"os/exec"
	"strings"
)

// seedPATH ensures the process PATH includes locations where Homebrew and
// common toolchains install binaries. Apps launched from Finder (or via
// Homebrew Cask) inherit macOS's login PATH — usually just
// /usr/bin:/bin:/usr/sbin:/sbin — which is missing every third-party
// converter we depend on (pandoc, soffice, ffmpeg, asciidoctor-pdf,
// pdftotext, xelatex, …).
//
// Called from main() before plugin.Load() so every subsequent probe sees
// the right binaries. Idempotent — safe to call twice.
func seedPATH() {
	// Ordered by preference — first hit wins. Include both Apple Silicon
	// and Intel Homebrew prefixes so the same binary lays out where the
	// tools already are on this box.
	extras := []string{
		"/opt/homebrew/bin",           // Apple Silicon Homebrew
		"/opt/homebrew/sbin",          //   "
		"/usr/local/bin",              // Intel Homebrew
		"/usr/local/sbin",             //   "
		"/Library/TeX/texbin",         // MacTeX / BasicTeX
		"/Library/Frameworks/Python.framework/Versions/Current/bin", // python.org
	}

	// Also honor whatever /etc/paths and /etc/paths.d/* say. This is what
	// login shells' `path_helper` does and it's the canonical macOS source
	// of truth for "what should be on PATH". Wrapping in a subshell keeps
	// the invocation portable across zsh/bash.
	if out, err := exec.Command("/usr/libexec/path_helper", "-s").Output(); err == nil {
		// Output looks like: PATH="/usr/local/bin:/usr/bin:..."; export PATH;
		s := string(out)
		if start := strings.Index(s, `PATH="`); start >= 0 {
			rest := s[start+len(`PATH="`):]
			if end := strings.Index(rest, `"`); end >= 0 {
				for _, p := range strings.Split(rest[:end], ":") {
					extras = append(extras, p)
				}
			}
		}
	}

	// Also try the user's login-shell environment. Users who install
	// tools into locations not covered by path_helper (e.g., rbenv,
	// pyenv, custom prefixes) usually put them on PATH via ~/.zshrc /
	// ~/.bash_profile. A single interactive-login shell invocation
	// yields the aggregate. Best-effort — if the shell isn't zsh or the
	// user's rcfile has side effects, we degrade gracefully.
	if shell := os.Getenv("SHELL"); shell != "" {
		if out, err := exec.Command(shell, "-l", "-c", "printf %s \"$PATH\"").Output(); err == nil {
			for _, p := range strings.Split(strings.TrimSpace(string(out)), ":") {
				extras = append(extras, p)
			}
		}
	}

	// Merge unique, preserving order, with the current PATH last so
	// anything already present stays reachable.
	seen := map[string]bool{}
	var out []string
	for _, p := range extras {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range strings.Split(os.Getenv("PATH"), ":") {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	_ = os.Setenv("PATH", strings.Join(out, ":"))
}
