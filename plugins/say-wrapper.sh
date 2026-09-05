#!/usr/bin/env bash
# say-wrapper.sh IN_TXT OUT_PATH VOICE SPEED
# Speed here is a Kokoro-style 0.5..2.0 multiplier. `say -r` takes words/min,
# with ~180 wpm treated as "normal" — we convert accordingly.
set -euo pipefail

in_txt="$1"
out_path="$2"
voice="${3:-Samantha}"
speed_mult="${4:-1.0}"

# words-per-minute: 180 * multiplier, rounded to int
wpm=$(awk -v m="$speed_mult" 'BEGIN{printf "%d", 180 * m}')

ext="${out_path##*.}"
ext="$(printf '%s' "$ext" | tr '[:upper:]' '[:lower:]')"

case "$ext" in
  aiff)
    exec say -o "$out_path" -v "$voice" -r "$wpm" -f "$in_txt"
    ;;
  mp3)
    tmp="$(mktemp -t mac-transformer-say).aiff"
    trap 'rm -f "$tmp"' EXIT
    say -o "$tmp" -v "$voice" -r "$wpm" -f "$in_txt"
    afconvert -f mp4f -d aac "$tmp" "${out_path%.mp3}.m4a" >/dev/null 2>&1 || true
    # `afconvert` -> MP4/AAC is the best macOS can do without lame; if the
    # caller strictly needs .mp3 we ffmpeg-transcode if available.
    if [[ -x "$(command -v ffmpeg)" ]]; then
      ffmpeg -y -loglevel error -i "$tmp" -codec:a libmp3lame -b:a 128k "$out_path"
    else
      # Fallback: move the .m4a to .mp3 — technically wrong container but
      # most players tolerate. Kokoro is the recommended MP3 path.
      mv "${out_path%.mp3}.m4a" "$out_path"
    fi
    ;;
  *)
    echo "say-wrapper: unsupported extension .$ext" >&2
    exit 2
    ;;
esac
