package plugin

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TemplateContext is what plugin manifests can reference via {{name}} placeholders.
// Field names are the placeholder keys.
type TemplateContext struct {
	Input      string
	InputStem  string
	InputExt   string
	InputText  string
	Output     string
	OutputStem string
	OutputExt  string
	OutputDir  string
	PluginDir  string
	Voice      string
	Speed      float64
	Extra      map[string]string
}

func (c *TemplateContext) lookup(key string) (string, bool) {
	switch key {
	case "input":
		return c.Input, c.Input != ""
	case "input_stem":
		return c.InputStem, c.InputStem != ""
	case "input_ext":
		return c.InputExt, c.InputExt != ""
	case "input_text":
		return c.InputText, c.InputText != ""
	case "output":
		return c.Output, c.Output != ""
	case "output_stem":
		return c.OutputStem, c.OutputStem != ""
	case "output_ext":
		return c.OutputExt, c.OutputExt != ""
	case "output_dir", "outdir":
		return c.OutputDir, c.OutputDir != ""
	case "plugin_dir":
		return c.PluginDir, c.PluginDir != ""
	case "voice":
		return c.Voice, c.Voice != ""
	case "speed":
		return strconv.FormatFloat(c.Speed, 'f', -1, 64), c.Speed != 0
	}
	if c.Extra != nil {
		v, ok := c.Extra[key]
		return v, ok
	}
	return "", false
}

var tmplPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// Expand replaces {{name}} and {{name | filter[:arg]}} occurrences in tmpl
// using values from ctx. Supported filters:
//   - default:VALUE   use VALUE if the key is unset
//   - json            JSON-encode the resulting string (for embedding in JSON bodies)
//   - shell           single-quote-escape for shell contexts
//   - trim            trim surrounding whitespace
// An unknown filter is treated as identity; unknown keys with no default
// resolve to empty string.
func Expand(tmpl string, ctx *TemplateContext) string {
	return tmplPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		inner := tmplPattern.FindStringSubmatch(match)[1]
		parts := strings.Split(inner, "|")
		key := strings.TrimSpace(parts[0])
		val, ok := ctx.lookup(key)
		var filters []string
		if len(parts) > 1 {
			filters = parts[1:]
		}
		for _, f := range filters {
			f = strings.TrimSpace(f)
			name, arg, hasArg := f, "", false
			if colon := strings.IndexByte(f, ':'); colon >= 0 {
				name = strings.TrimSpace(f[:colon])
				arg = strings.TrimSpace(f[colon+1:])
				hasArg = true
			}
			switch name {
			case "default":
				if !ok && hasArg {
					val = arg
					ok = true
				}
			case "json":
				b, _ := json.Marshal(val)
				val = string(b)
			case "shell":
				val = "'" + strings.ReplaceAll(val, "'", `'\''`) + "'"
			case "trim":
				val = strings.TrimSpace(val)
			}
		}
		return val
	})
}

// ExpandJSON recursively walks a decoded JSON structure and expands string
// leaves as templates. Used for body_json expansion.
func ExpandJSON(v any, ctx *TemplateContext) any {
	switch x := v.(type) {
	case string:
		s := Expand(x, ctx)
		// If the whole value is a template that resolved to a number-like
		// string ({{speed}}), coerce it so JSON stays typed correctly.
		if n, err := strconv.ParseFloat(s, 64); err == nil && looksNumeric(x) {
			return n
		}
		return s
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = ExpandJSON(vv, ctx)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = ExpandJSON(vv, ctx)
		}
		return out
	default:
		return v
	}
}

func looksNumeric(tmpl string) bool {
	inner := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(tmpl), "}}"), "{{"))
	// crude: if the placeholder key is speed or ends with _number, treat as numeric
	key := strings.TrimSpace(strings.SplitN(inner, "|", 2)[0])
	return key == "speed" || strings.HasSuffix(key, "_number")
}

// Errorf is a tiny wrapper so callers can produce plugin-tagged errors.
func Errorf(m *Manifest, format string, args ...any) error {
	return fmt.Errorf("plugin %q: "+format, append([]any{m.Name}, args...)...)
}
