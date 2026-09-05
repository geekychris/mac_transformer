package core

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// tagAudioFile sets ID3v2 tags on an MP3 (or common-container audio) via
// ffmpeg's -metadata option. Silent no-op if ffmpeg isn't installed — the
// file's still valid, just untagged.
//
// track / total are the current chapter number and total chapter count.
// album is the parent document's stem. artist defaults to "Mac Transformer"
// so tracks group correctly in Music.app.
func tagAudioFile(ctx context.Context, path, title, album, artist, genre string, track, total int) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil // no ffmpeg, no tags — not fatal
	}
	// Keep the original extension so ffmpeg autodetects the output format
	// correctly (foo.aiff.tmp would fall back to a wrong container).
	ext := path[strings.LastIndex(path, "."):]
	tmp := strings.TrimSuffix(path, ext) + ".tagging" + ext

	args := []string{"-y", "-i", path, "-c", "copy"}
	add := func(k, v string) {
		if v == "" {
			return
		}
		args = append(args, "-metadata", fmt.Sprintf("%s=%s", k, v))
	}
	add("title", title)
	add("album", album)
	add("artist", artist)
	add("album_artist", artist)
	add("genre", genre)
	if track > 0 && total > 0 {
		args = append(args, "-metadata", fmt.Sprintf("track=%d/%d", track, total))
	} else if track > 0 {
		args = append(args, "-metadata", fmt.Sprintf("track=%d", track))
	}
	// ffmpeg needs to know the output format. Preserve the input extension.
	args = append(args, tmp)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg tag: %s: %s", err, strings.TrimSpace(string(out)))
	}
	// Atomic replace.
	return exec.Command("mv", tmp, path).Run()
}
