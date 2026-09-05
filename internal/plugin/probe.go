package plugin

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Probe checks whether every Requirement of a plugin is currently satisfied.
// Returns nil if the plugin is usable, or an error describing the first
// unmet requirement. Probes are best-effort and fast (network probes have
// a 2s timeout) — they're rerun on demand from the /plugins endpoint.
func Probe(ctx context.Context, m *Manifest) error {
	for _, req := range m.Requires {
		if err := probeOne(ctx, req); err != nil {
			return fmt.Errorf("%s: %w", req.Kind, err)
		}
	}
	return nil
}

func probeOne(ctx context.Context, r Requirement) error {
	switch r.Kind {
	case "binary":
		if r.Name == "" {
			return fmt.Errorf("binary requirement missing name")
		}
		if _, err := exec.LookPath(r.Name); err != nil {
			return fmt.Errorf("%q not on PATH", r.Name)
		}
		return nil
	case "app-bundle":
		if r.BundleID == "" {
			return fmt.Errorf("app-bundle requirement missing bundle_id")
		}
		// osascript: `id of application "..."` fails if not installed
		out, err := exec.CommandContext(ctx, "osascript", "-e",
			fmt.Sprintf(`id of application id %q`, r.BundleID)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("bundle %s not installed: %s", r.BundleID, strings.TrimSpace(string(out)))
		}
		return nil
	case "http":
		if r.URL == "" {
			return fmt.Errorf("http requirement missing url")
		}
		expect := r.ExpectStatus
		if expect == 0 {
			expect = 200
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(probeCtx, "GET", r.URL, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("cannot reach %s: %w", r.URL, err)
		}
		resp.Body.Close()
		if resp.StatusCode != expect {
			return fmt.Errorf("%s returned %d (want %d)", r.URL, resp.StatusCode, expect)
		}
		return nil
	case "gem":
		if _, err := exec.LookPath(r.Name); err != nil {
			return fmt.Errorf("gem executable %q not on PATH", r.Name)
		}
		return nil
	case "file":
		if r.Path == "" {
			return fmt.Errorf("file requirement missing path")
		}
		if err := statFile(r.Path); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown requirement kind %q", r.Kind)
	}
}
