package plugin

import "testing"

func TestExpand_BasicSubstitutions(t *testing.T) {
	ctx := &TemplateContext{
		Input:      "/a/b/foo.pdf",
		InputStem:  "foo",
		InputExt:   "pdf",
		Output:     "/x/y/foo.txt",
		OutputStem: "foo",
		OutputExt:  "txt",
		Voice:      "af_bella",
		Speed:      1.2,
	}
	cases := []struct{ in, want string }{
		{"{{input}}", "/a/b/foo.pdf"},
		{"{{output}}", "/x/y/foo.txt"},
		{"{{input_stem}}.{{output_ext}}", "foo.txt"},
		{"{{voice}}", "af_bella"},
		{"{{speed}}", "1.2"},
	}
	for _, c := range cases {
		got := Expand(c.in, ctx)
		if got != c.want {
			t.Errorf("Expand(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExpand_DefaultFilter(t *testing.T) {
	ctx := &TemplateContext{}
	if got := Expand("{{missing | default:fallback}}", ctx); got != "fallback" {
		t.Errorf("default filter: got %q", got)
	}
	ctx.Voice = "af_bella"
	if got := Expand("{{voice | default:af_default}}", ctx); got != "af_bella" {
		t.Errorf("default should skip when set: got %q", got)
	}
}

func TestExpand_JSONFilter(t *testing.T) {
	ctx := &TemplateContext{Input: `a "quoted" b`}
	got := Expand(`{{input | json}}`, ctx)
	want := `"a \"quoted\" b"`
	if got != want {
		t.Errorf("json filter: got %q, want %q", got, want)
	}
}

func TestExpandJSON_CoercesSpeed(t *testing.T) {
	ctx := &TemplateContext{Speed: 1.4}
	m := map[string]any{"speed": "{{speed}}", "voice": "{{voice | default:af_bella}}"}
	out := ExpandJSON(m, ctx)
	mo, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("not a map: %T", out)
	}
	if n, ok := mo["speed"].(float64); !ok || n != 1.4 {
		t.Errorf("speed not coerced to number: %v %T", mo["speed"], mo["speed"])
	}
	if mo["voice"] != "af_bella" {
		t.Errorf("voice default not applied: %v", mo["voice"])
	}
}
