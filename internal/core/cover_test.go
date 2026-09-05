package core

import (
	"bytes"
	"image/png"
	"testing"
)

func TestGenerateCoverImage_ProducesValidPNG(t *testing.T) {
	data, err := generateCoverImage("A Test Book Title")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(data) < 100 {
		t.Errorf("too small: %d bytes", len(data))
	}
	// Decode to confirm it's a valid PNG.
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 600 {
		t.Errorf("wrong dimensions: %v", img.Bounds())
	}
}

func TestGenerateCoverImage_DeterministicForSameTitle(t *testing.T) {
	a, _ := generateCoverImage("Same Title")
	b, _ := generateCoverImage("Same Title")
	if !bytes.Equal(a, b) {
		t.Errorf("cover should be deterministic for same title")
	}
}

func TestGenerateCoverImage_DifferentTitles(t *testing.T) {
	a, _ := generateCoverImage("Title A")
	b, _ := generateCoverImage("Title B")
	if bytes.Equal(a, b) {
		t.Errorf("different titles should produce different covers")
	}
}
