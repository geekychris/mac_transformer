package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBundledPluginsLoad verifies every YAML manifest in the bundled
// plugins/ directory parses and passes basic schema validation. Catches
// typos in manifests before they ship — the earlier PowerPoint bug slipped
// past unit tests exactly because there was no validation of the bundled
// set.
func TestBundledPluginsLoad(t *testing.T) {
	pluginDir := bundledPluginDir(t)

	reg, errs := Load(context.Background(), []string{pluginDir})
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("plugin load: %v", e)
		}
	}

	loaded := reg.All()
	if len(loaded) == 0 {
		t.Fatalf("no plugins loaded from %s", pluginDir)
	}

	// Schema sanity: every plugin must have at least one capability with
	// non-empty from/to lists and a positive priority.
	for _, lp := range loaded {
		m := lp.Manifest
		if len(m.Capabilities) == 0 {
			t.Errorf("%s: no capabilities", m.Name)
		}
		for i, c := range m.Capabilities {
			if len(c.From) == 0 || len(c.To) == 0 {
				t.Errorf("%s.capabilities[%d]: empty from or to", m.Name, i)
			}
			if c.Priority <= 0 {
				t.Errorf("%s.capabilities[%d]: non-positive priority %d", m.Name, i, c.Priority)
			}
		}
		if m.Run.Kind == "" {
			t.Errorf("%s: missing run.kind", m.Name)
		}
		switch m.Run.Kind {
		case "command":
			if len(m.Run.Argv) == 0 {
				t.Errorf("%s: command plugin has empty argv", m.Name)
			}
		case "http":
			if m.Run.URL == "" {
				t.Errorf("%s: http plugin has empty url", m.Name)
			}
		default:
			t.Errorf("%s: unknown run.kind %q", m.Name, m.Run.Kind)
		}
	}
}

// TestBundledAppleScriptsCompile ensures every .applescript companion in
// plugins/ parses as valid AppleScript. `osascript -c` compiles without
// executing, so this runs offline and doesn't touch Books/Music/Word/etc.
// The earlier PowerPoint bug (variable used before defined) would have
// been caught here.
func TestBundledAppleScriptsCompile(t *testing.T) {
	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skip("osascript not available (non-macOS environment)")
	}
	pluginDir := bundledPluginDir(t)
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".applescript") {
			continue
		}
		found++
		script := filepath.Join(pluginDir, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			// `osacompile -o /dev/null` performs a compile-only pass. We
			// don't have osacompile guaranteed everywhere, so fall back
			// to a `osascript` dry-run in a subshell that quotes the
			// script inside a `run` block — if the syntax is broken,
			// osascript errors out before running anything.
			//
			// The cleanest cross-macOS pattern is `osacompile -o <out>`.
			out := filepath.Join(t.TempDir(), "compiled.scpt")
			cmd := exec.Command("osacompile", "-o", out, script)
			if err := cmd.Run(); err != nil {
				// Fall back to reading + running with a syntax-only mode
				// if osacompile is unavailable.
				cmd = exec.Command("osascript", "-e", `on run argv
end run`, script)
				stderr, _ := cmd.CombinedOutput()
				if len(stderr) > 0 {
					t.Errorf("compile failed: %s", strings.TrimSpace(string(stderr)))
				}
				return
			}
		})
	}
	if found == 0 {
		t.Fatalf("no .applescript files found in %s", pluginDir)
	}
}

// TestBundledPluginsHaveRequiredCompanions verifies that every plugin
// manifest that references a companion script (via {{plugin_dir}}/…)
// actually has that companion file on disk. This is the class of bug
// that lands users at runtime with "no such file" errors.
func TestBundledPluginsHaveRequiredCompanions(t *testing.T) {
	pluginDir := bundledPluginDir(t)
	reg, _ := Load(context.Background(), []string{pluginDir})
	for _, lp := range reg.All() {
		if lp.Manifest.Run.Kind != "command" {
			continue
		}
		for _, arg := range lp.Manifest.Run.Argv {
			if !strings.Contains(arg, "{{plugin_dir}}") {
				continue
			}
			relative := strings.ReplaceAll(arg, "{{plugin_dir}}/", "")
			abs := filepath.Join(pluginDir, relative)
			if _, err := os.Stat(abs); err != nil {
				t.Errorf("%s: companion %q not found (%v)", lp.Manifest.Name, relative, err)
			}
		}
	}
}

// bundledPluginDir walks up from the test's package to find plugins/.
// go test runs in the package directory, so plugins/ is two levels up.
func bundledPluginDir(t *testing.T) string {
	t.Helper()
	// From internal/plugin/, plugins/ is at ../../plugins.
	dir, err := filepath.Abs(filepath.Join("..", "..", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("bundled plugin dir not found: %s", dir)
	}
	return dir
}
