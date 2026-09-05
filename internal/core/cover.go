package core

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// generateCoverImage draws a simple album-art PNG for a given title. The
// background color is derived from a hash of the title so the same doc
// always gets the same color and different docs are visually distinct.
// Returns the PNG bytes; caller writes to disk or hands to ffmpeg.
//
// Dead simple visual: solid color background, title wrapped in the middle,
// small "Mac Transformer" credit at the bottom. Not going for design
// awards — for identification at a glance in Music.app grid views.
func generateCoverImage(title string) ([]byte, error) {
	const w, h = 600, 600
	bg := colorFromString(title)
	fg := contrastingColor(bg)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Title text — center vertically, wrap at ~24 chars/line.
	lines := wrapText(title, 24)
	if len(lines) > 6 {
		lines = lines[:6]
	}
	face := basicfont.Face7x13
	lineHeight := 40
	totalH := len(lines) * lineHeight
	startY := (h - totalH) / 2

	for i, line := range lines {
		drawCenteredText(img, line, w/2, startY+i*lineHeight, face, fg, 3)
	}

	// Small credit at the bottom.
	drawCenteredText(img, "Mac Transformer", w/2, h-30, face, fg, 1)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// embedMP3Cover uses ffmpeg to attach a PNG cover to an MP3. Silent no-op
// if ffmpeg isn't installed or the input isn't an MP3.
func embedMP3Cover(audioPath string, coverPNG []byte) error {
	if strings.ToLower(filepath.Ext(audioPath)) != ".mp3" {
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil
	}
	// Write cover to temp.
	tmpCover, err := os.CreateTemp("", "mac-transformer-cover-*.png")
	if err != nil {
		return err
	}
	defer os.Remove(tmpCover.Name())
	if _, err := tmpCover.Write(coverPNG); err != nil {
		tmpCover.Close()
		return err
	}
	tmpCover.Close()

	// Rewrite MP3 with attached image as ID3v2 APIC frame.
	tmp := strings.TrimSuffix(audioPath, ".mp3") + ".cover.mp3"
	cmd := exec.Command("ffmpeg", "-y",
		"-i", audioPath,
		"-i", tmpCover.Name(),
		"-map", "0:0", "-map", "1:0",
		"-c", "copy",
		"-id3v2_version", "3",
		"-metadata:s:v", "title=Album cover",
		"-metadata:s:v", "comment=Cover (front)",
		tmp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg cover: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Rename(tmp, audioPath)
}

// --- helpers ---

// colorFromString hashes the input and turns the first 3 bytes into a
// muted HSL-ish color. Same input -> same color.
func colorFromString(s string) color.Color {
	h := sha1.Sum([]byte(s))
	// Bias toward mid-brightness by clamping the range.
	r := uint8(40 + int(h[0])%160)
	g := uint8(40 + int(h[1])%160)
	b := uint8(40 + int(h[2])%160)
	return color.RGBA{r, g, b, 255}
}

func contrastingColor(bg color.Color) color.Color {
	r, g, b, _ := bg.RGBA()
	// Perceptual luminance; if bg is dark, use white text.
	l := 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(b>>8)
	if l < 128 {
		return color.RGBA{255, 255, 255, 255}
	}
	return color.RGBA{20, 20, 20, 255}
}

// wrapText greedy-wraps into ~maxChars lines, breaking on spaces.
func wrapText(s string, maxChars int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > maxChars {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = cur + " " + w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// drawCenteredText draws s centered horizontally at x, baseline y, scaled
// by `scale` (basicfont is fixed 7x13 so we hand-repeat pixels to enlarge).
func drawCenteredText(img *image.RGBA, s string, x, y int, face font.Face, col color.Color, scale int) {
	// Measure.
	measurer := &font.Drawer{Face: face}
	width := measurer.MeasureString(s).Ceil() * scale
	startX := x - width/2

	for _, r := range s {
		dr, mask, _, advance, ok := face.Glyph(fixed.Point26_6{}, r)
		if !ok {
			startX += 7 * scale // basicfont advance ~7px
			continue
		}
		_ = dr
		gRect := mask.Bounds()
		for py := 0; py < gRect.Dy(); py++ {
			for px := 0; px < gRect.Dx(); px++ {
				if _, _, _, a := mask.At(gRect.Min.X+px, gRect.Min.Y+py).RGBA(); a > 0 {
					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							img.Set(startX+px*scale+sx, y+(py-13)*scale+sy, col)
						}
					}
				}
			}
		}
		startX += advance.Ceil() * scale
	}
}
