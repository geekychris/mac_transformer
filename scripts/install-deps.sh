#!/usr/bin/env bash
# install-deps.sh — install optional runtime dependencies for Mac Transformer.
#
# Each dep is prompted individually — press Enter to accept, N to skip.
# Anything you skip just marks the corresponding plugin unavailable until
# you install it later; the app still runs.
#
# Non-interactive mode: MT_ASSUME_YES=1 ./scripts/install-deps.sh
# Skip prompts and install everything.

set -euo pipefail

MT_ASSUME_YES="${MT_ASSUME_YES:-0}"

if ! command -v brew >/dev/null 2>&1; then
  echo "This script uses Homebrew to install dependencies." >&2
  echo "Install Homebrew first from https://brew.sh, then re-run." >&2
  exit 1
fi

ask() {
  local prompt="$1"
  if [[ "$MT_ASSUME_YES" == "1" ]]; then
    return 0
  fi
  read -r -p "$prompt [Y/n] " reply
  case "$reply" in
    N|n|no|No) return 1 ;;
    *)         return 0 ;;
  esac
}

install_brew() {
  local pkg="$1"
  local kind="${2:-formula}"  # formula or cask
  if [[ "$kind" == "cask" ]]; then
    if brew list --cask "$pkg" >/dev/null 2>&1; then
      echo "  already installed: $pkg"
      return
    fi
    brew install --cask "$pkg"
  else
    if brew list "$pkg" >/dev/null 2>&1; then
      echo "  already installed: $pkg"
      return
    fi
    brew install "$pkg"
  fi
}

install_gem() {
  local gem="$1"
  if command -v "$gem" >/dev/null 2>&1; then
    echo "  already installed: $gem"
    return
  fi
  gem install "$gem"
}

echo "==> Mac Transformer optional dependency installer"
echo

# --- Pandoc — markup → PDF/txt ---
if ask "Install Pandoc? (needed for md/adoc/rst/html → txt/pdf conversion)"; then
  install_brew pandoc
fi

# --- Poppler (pdftotext) — PDF → txt fallback ---
if ask "Install poppler? (provides pdftotext for PDF text extraction)"; then
  install_brew poppler
fi

# --- LibreOffice — Office → PDF/txt ---
if ask "Install LibreOffice? (~700MB — best-fidelity fallback for docx/pptx → PDF when Word/PowerPoint aren't available)"; then
  install_brew libreoffice cask
fi

# --- BasicTeX — Pandoc's PDF pipeline ---
if ask "Install BasicTeX? (~100MB — required for pandoc's Markdown → PDF pipeline)"; then
  install_brew basictex cask
  echo "  note: log out and back in (or 'eval \"\$(/usr/libexec/path_helper)\"') to pick up latex on PATH"
fi

# --- ffmpeg — audio metadata + cover art embedding ---
if ask "Install ffmpeg? (needed for ID3 tags on MP3 audio and embedded cover art)"; then
  install_brew ffmpeg
fi

# --- asciidoctor-pdf — high-fidelity .adoc → PDF ---
if ask "Install asciidoctor-pdf? (best-in-class AsciiDoc → PDF)"; then
  install_gem asciidoctor-pdf
fi

# --- Kokoro (external repo) ---
if ask "Install Kokoro TTS runtime? (local neural TTS — runs at http://127.0.0.1:8770)"; then
  if [[ -d "$HOME/.kokoro/src" ]]; then
    echo "  Kokoro already installed at ~/.kokoro/src"
  else
    echo "  Running: curl -fsSL https://raw.githubusercontent.com/geekychris/kokoro_runtime/main/scripts/install.sh | KOKORO_AUTO_INSTALL=1 bash"
    curl -fsSL https://raw.githubusercontent.com/geekychris/kokoro_runtime/main/scripts/install.sh | KOKORO_AUTO_INSTALL=1 bash
  fi
fi

echo
echo "==> Done. Launch Mac Transformer and check the Plugins tab to confirm"
echo "    which plugins are now available. Any that still show 'unavailable'"
echo "    will list the missing dependency in the reason field."
