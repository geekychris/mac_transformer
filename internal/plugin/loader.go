package plugin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// LoadedPlugin bundles a parsed manifest with the outcome of its availability probe.
type LoadedPlugin struct {
	Manifest  *Manifest
	Available bool
	Reason    string // populated when Available is false
}

// Registry is an in-memory catalog of loaded plugins. It's safe for concurrent
// read access from HTTP handlers.
type Registry struct {
	mu      sync.RWMutex
	plugins []*LoadedPlugin
}

// Load walks the given plugin directories (in order — later dirs override
// earlier ones by name) and returns a Registry populated with everything that
// parsed successfully. Parse errors are logged but don't stop the load.
func Load(ctx context.Context, dirs []string) (*Registry, []error) {
	seen := map[string]*LoadedPlugin{}
	var errs []error

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("read %s: %w", dir, err))
			}
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
				continue
			}
			path := filepath.Join(dir, name)
			m, err := parseManifest(path)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", path, err))
				continue
			}
			seen[m.Name] = &LoadedPlugin{Manifest: m}
		}
	}

	// Now probe each plugin.
	list := make([]*LoadedPlugin, 0, len(seen))
	for _, lp := range seen {
		if err := Probe(ctx, lp.Manifest); err != nil {
			lp.Available = false
			lp.Reason = err.Error()
		} else {
			lp.Available = true
		}
		list = append(list, lp)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Manifest.Name < list[j].Manifest.Name
	})

	return &Registry{plugins: list}, errs
}

func parseManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(false)
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m.Name == "" {
		return nil, fmt.Errorf("manifest missing name")
	}
	if len(m.Capabilities) == 0 {
		return nil, fmt.Errorf("manifest %s has no capabilities", m.Name)
	}
	if m.Run.Kind == "" {
		return nil, fmt.Errorf("manifest %s has no run.kind", m.Name)
	}
	m.Source = path
	m.Dir = filepath.Dir(path)
	return &m, nil
}

// All returns a copy of the loaded plugin list.
func (r *Registry) All() []*LoadedPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*LoadedPlugin, len(r.plugins))
	copy(out, r.plugins)
	return out
}

// Available returns plugins whose probes passed.
func (r *Registry) Available() []*LoadedPlugin {
	all := r.All()
	out := make([]*LoadedPlugin, 0, len(all))
	for _, lp := range all {
		if lp.Available {
			out = append(out, lp)
		}
	}
	return out
}

// Match returns available plugins that can convert from -> to, ordered
// highest priority first. Format strings are lowercase extensions without
// leading dots ("docx", "pdf").
func (r *Registry) Match(from, to string) []*LoadedPlugin {
	type scored struct {
		lp       *LoadedPlugin
		priority int
	}
	var candidates []scored
	for _, lp := range r.Available() {
		for _, cap := range lp.Manifest.Capabilities {
			if containsFold(cap.From, from) && containsFold(cap.To, to) {
				candidates = append(candidates, scored{lp, cap.Priority})
				break
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})
	out := make([]*LoadedPlugin, len(candidates))
	for i, c := range candidates {
		out[i] = c.lp
	}
	return out
}

// ByName returns a specific plugin (available or not) by name.
func (r *Registry) ByName(name string) *LoadedPlugin {
	for _, lp := range r.All() {
		if lp.Manifest.Name == name {
			return lp
		}
	}
	return nil
}

// Reload re-probes every plugin without re-reading manifests. Useful when
// external state changes (e.g. kokoro-server just started).
func (r *Registry) Reload(ctx context.Context) {
	all := r.All()
	for _, lp := range all {
		if err := Probe(ctx, lp.Manifest); err != nil {
			lp.Available = false
			lp.Reason = err.Error()
		} else {
			lp.Available = true
			lp.Reason = ""
		}
	}
}

func containsFold(hay []string, needle string) bool {
	needle = strings.ToLower(needle)
	for _, h := range hay {
		if strings.ToLower(h) == needle {
			return true
		}
	}
	return false
}

// helper for probe.go
func statFile(path string) error {
	if fi, err := os.Stat(path); err != nil {
		return err
	} else if fi.Mode()&fs.ModeType != 0 {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}
