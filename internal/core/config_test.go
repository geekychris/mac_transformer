package core

import "testing"

func TestConfigMerge_FillsDefaults(t *testing.T) {
	base := Defaults()
	// Override only some fields.
	override := Config{
		DefaultKokoroVoice: "am_adam",
		OutputDir:          "/custom/out",
	}
	got := merge(base, override)

	if got.DefaultKokoroVoice != "am_adam" {
		t.Errorf("voice override lost: %s", got.DefaultKokoroVoice)
	}
	if got.OutputDir != "/custom/out" {
		t.Errorf("output dir override lost: %s", got.OutputDir)
	}
	// Untouched field kept from base.
	if got.DefaultTTSEngine != base.DefaultTTSEngine {
		t.Errorf("base engine lost")
	}
	if got.Port != base.Port {
		t.Errorf("base port lost: %d", got.Port)
	}
}

func TestConfigDefaults_UsableOutOfBox(t *testing.T) {
	d := Defaults()
	if d.Port == 0 {
		t.Errorf("no default port")
	}
	if d.OutputDir == "" {
		t.Errorf("no default output dir")
	}
	if d.DefaultLayout == "" {
		t.Errorf("no default layout")
	}
	if d.DefaultTTSEngine == "" {
		t.Errorf("no default TTS engine")
	}
}
