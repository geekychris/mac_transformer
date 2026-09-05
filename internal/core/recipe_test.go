package core

import "testing"

func TestApplyRecipe_OverridesOutputs(t *testing.T) {
	base := Request{InputPath: "/tmp/foo.docx"}
	recipe := Recipe{
		Name: "Audiobook",
		Request: Request{
			Outputs: []OutputKind{"audio"},
			TTS:     TTSOptions{Engine: "kokoro", Voice: "af_bella", Speed: 1.2},
		},
	}
	got := ApplyRecipe(base, recipe)
	if len(got.Outputs) != 1 || got.Outputs[0] != "audio" {
		t.Errorf("outputs not applied: %v", got.Outputs)
	}
	if got.TTS.Voice != "af_bella" {
		t.Errorf("voice not applied: %s", got.TTS.Voice)
	}
	// InputPath should NOT be overwritten by recipe (the point).
	if got.InputPath != "/tmp/foo.docx" {
		t.Errorf("recipe stomped InputPath: %s", got.InputPath)
	}
}

func TestApplyRecipe_PreservesBaseFields(t *testing.T) {
	base := Request{
		InputPath: "/tmp/foo.md",
		Outputs:   []OutputKind{"pdf"},
	}
	recipe := Recipe{
		Name: "Publish to Books",
		Request: Request{
			PublishToBooks: boolPtr(true),
		},
	}
	got := ApplyRecipe(base, recipe)
	// Recipe adds publish flag; existing Outputs stay.
	if got.PublishToBooks == nil || !*got.PublishToBooks {
		t.Errorf("publish flag not applied")
	}
	if len(got.Outputs) != 1 || got.Outputs[0] != "pdf" {
		t.Errorf("outputs stomped: %v", got.Outputs)
	}
}

func TestApplyRecipe_TagsReplace(t *testing.T) {
	base := Request{Tags: []string{"BaseTag"}}
	recipe := Recipe{Request: Request{Tags: []string{"RecipeTag"}}}
	got := ApplyRecipe(base, recipe)
	if len(got.Tags) != 1 || got.Tags[0] != "RecipeTag" {
		t.Errorf("tags not replaced: %v", got.Tags)
	}
}

func TestApplyRecipe_AudioByChapter(t *testing.T) {
	base := Request{InputPath: "/tmp/foo.md"}
	recipe := Recipe{Request: Request{AudioByChapter: boolPtr(true)}}
	got := ApplyRecipe(base, recipe)
	if got.AudioByChapter == nil || !*got.AudioByChapter {
		t.Errorf("audio_by_chapter not applied")
	}
}

func boolPtr(b bool) *bool { return &b }
