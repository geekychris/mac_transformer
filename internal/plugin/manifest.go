package plugin

// Manifest is the on-disk representation of a converter plugin.
// A plugin declares:
//   - one or more Capabilities (input format -> output format at some priority)
//   - one or more Requirements that must be satisfied for the plugin to be usable
//   - a Run block describing how to actually perform the conversion
//
// Manifests are loaded from bundled plugins/ and from ~/.mac-transformer/plugins/.
type Manifest struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	Version      int           `yaml:"version"`
	Capabilities []Capability  `yaml:"capabilities"`
	Requires     []Requirement `yaml:"requires"`
	Run          Run           `yaml:"run"`

	// Filled at load time.
	Dir      string `yaml:"-"`
	Source   string `yaml:"-"` // absolute path to the manifest file
	Bundled  bool   `yaml:"-"` // true if shipped with the app
}

// Capability is a (from formats -> to formats) mapping at a given priority.
// Higher priority wins when multiple plugins can perform the same conversion.
type Capability struct {
	From     []string `yaml:"from"`
	To       []string `yaml:"to"`
	Priority int      `yaml:"priority"`
}

// Requirement is a precondition checked once at load time. If any Requirement
// fails, the plugin is loaded but marked unavailable (with a reason surfaced
// in /plugins so the user can see why).
type Requirement struct {
	Kind         string `yaml:"kind"` // binary | app-bundle | http | gem | file
	Name         string `yaml:"name"`
	BundleID     string `yaml:"bundle_id"`
	URL          string `yaml:"url"`
	ExpectStatus int    `yaml:"expect_status"`
	Path         string `yaml:"path"`
}

// Run describes how to invoke the converter. Two kinds are supported today:
//   - command:  spawn a subprocess with argv (template-expanded)
//   - http:     POST/GET/etc to a URL with a template-expanded body
type Run struct {
	Kind string `yaml:"kind"` // command | http

	// command kind
	Argv       []string          `yaml:"argv"`
	Env        map[string]string `yaml:"env"`
	Stdin      string            `yaml:"stdin"`
	Workdir    string            `yaml:"workdir"`
	OutputFrom string            `yaml:"output_from"` // file (default) | stdout
	Produces   string            `yaml:"produces"`    // where the tool wrote its output, if not {{output}}

	// http kind
	Method         string            `yaml:"method"`
	URL            string            `yaml:"url"`
	Headers        map[string]string `yaml:"headers"`
	BodyJSON       map[string]any    `yaml:"body_json"`
	BodyTemplate   string            `yaml:"body_template"`
	ResponseToFile string            `yaml:"response_to_file"`

	TimeoutSeconds int `yaml:"timeout_seconds"`
}
