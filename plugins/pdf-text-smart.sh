#!/usr/bin/env bash
# pdf-text-smart.sh IN.pdf OUT.txt
#
# Tries pdftotext first. If the resulting text is empty or trivially short,
# falls back to macOS Vision OCR (ocr-pdf.swift). This lets scanned PDFs
# still produce usable text without the user having to know which is which.
set -euo pipefail

in="$1"
out="$2"
plugin_dir="$(cd "$(dirname "$0")" && pwd)"

# Threshold: below this many non-whitespace characters, treat as "no text
# layer" and run OCR instead. Chosen empirically — cover pages and title
# pages can produce a few dozen chars but real content-bearing PDFs
# produce hundreds even for short docs.
threshold=100

pdftotext -layout -enc UTF-8 "$in" "$out" 2>/dev/null || true

if [[ -f "$out" ]]; then
  # Count non-whitespace chars.
  count=$(tr -d ' \t\n\r' < "$out" | wc -c | tr -d ' ')
  if [[ "$count" -ge "$threshold" ]]; then
    exit 0
  fi
fi

echo "pdftotext produced too little text; falling back to OCR" >&2
swift "$plugin_dir/ocr-pdf.swift" "$in" "$out"
