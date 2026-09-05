#!/usr/bin/env bash
# Mac Transformer installer.
#
# Builds the app from source, drops the Finder Quick Action into
# ~/Library/Services, and (optionally) registers the MCP endpoint with
# Claude Code.
#
# Environment knobs:
#   MT_INSTALL_QUICK_ACTION=0     skip installing the Automator Quick Action
#   MT_INSTALL_MCP=0              skip `claude mcp add`
#   MT_BUILD=0                    don't rebuild if the .app already exists
#   MT_INSTALL_TO_APPLICATIONS=0  keep the .app in ./bin (default: install into /Applications)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "==> Mac Transformer installer (from $REPO_ROOT)"

# --- prereqs ---
missing=()
command -v go >/dev/null 2>&1 || missing+=("go")
command -v npm >/dev/null 2>&1 || missing+=("npm")
command -v pandoc >/dev/null 2>&1 || echo "note: pandoc not found — markdown/adoc PDF/text plugins will be inactive"
command -v soffice >/dev/null 2>&1 || echo "note: soffice not found — LibreOffice conversion plugin will be inactive"
if [[ ${#missing[@]} -gt 0 ]]; then
  echo "error: missing required tools: ${missing[*]}" >&2
  echo "  install Go: https://go.dev/dl/  |  install npm via Homebrew: brew install node" >&2
  exit 1
fi

# --- wails3 CLI (idempotent) ---
if ! command -v wails3 >/dev/null 2>&1 && [[ ! -x "$HOME/go/bin/wails3" ]]; then
  echo "==> Installing Wails v3 CLI"
  go install github.com/wailsapp/wails/v3/cmd/wails3@latest
fi
export PATH="$PATH:$HOME/go/bin"

# --- frontend + bindings + .app bundle ---
if [[ "${MT_BUILD:-1}" != "0" ]]; then
  echo "==> Generating Wails bindings"
  wails3 generate bindings >/dev/null

  echo "==> Building .app bundle (frontend, Go binary, Info.plist)"
  # `wails3 package` invokes the darwin Taskfile which does:
  #   frontend build -> icons -> `go build` -> assemble Contents/ -> ad-hoc codesign
  rm -rf bin/mac-transformer.app
  wails3 package >/dev/null
fi

APP_BUNDLE="$REPO_ROOT/bin/mac-transformer.app"
echo "==> App bundle: $APP_BUNDLE"
echo "==> Bare binary: $APP_BUNDLE/Contents/MacOS/mac-transformer"

# --- Register with Launch Services so Finder learns the doc types ---
if [[ -d "$APP_BUNDLE" ]]; then
  echo "==> Registering with Launch Services (Open With + document types)"
  lsregister="/System/Library/Frameworks/CoreServices.framework/Versions/A/Frameworks/LaunchServices.framework/Versions/A/Support/lsregister"
  "$lsregister" -f "$APP_BUNDLE" || echo "note: lsregister returned non-zero (Finder may still pick up the app after next login)"
fi

# --- Install into /Applications (required for Finder Open-With to show us) ---
#
# Launch Services quietly deprioritizes apps outside /Applications for Open
# With menus and per-file UTI lookups. Even with our bundle correctly
# registered, apps under ~/code/… get hidden or ranked below TextEdit &
# friends. Copying to /Applications is the only reliable fix.
#
# Set MT_INSTALL_TO_APPLICATIONS=0 to keep the app in bin/ (useful for
# development, but Open With won't work correctly).
if [[ "${MT_INSTALL_TO_APPLICATIONS:-1}" != "0" ]]; then
  echo "==> Installing to /Applications"
  # Unregister any stale copy first so LS doesn't hold on to old entries.
  "$lsregister" -u "/Applications/Mac Transformer.app" 2>/dev/null || true
  rm -rf "/Applications/Mac Transformer.app"
  cp -R "$APP_BUNDLE" "/Applications/Mac Transformer.app"
  "$lsregister" -f "/Applications/Mac Transformer.app" || \
    echo "note: lsregister returned non-zero"
  # Also drop the dev-only bin/ copy from LS so Open With doesn't show two.
  "$lsregister" -u "$APP_BUNDLE" 2>/dev/null || true
  APP_BUNDLE="/Applications/Mac Transformer.app"

  # Kick Finder so its per-file UTI cache gets rebuilt. Without this,
  # existing .adoc/.rst/.org files in Finder still show as generic-text
  # (or with no Open-With entry at all) until macOS notices the new UTIs.
  echo "==> Restarting Finder to refresh UTI cache"
  killall Finder 2>/dev/null || true
fi

# --- Finder Quick Action ---
if [[ "${MT_INSTALL_QUICK_ACTION:-1}" != "0" ]]; then
  dest="$HOME/Library/Services/MacTransformer.workflow"
  echo "==> Installing Finder Quick Action to $dest"
  rm -rf "$dest"
  cp -R automator/MacTransformer.workflow "$dest"
fi

# --- Claude Code MCP registration ---
if [[ "${MT_INSTALL_MCP:-1}" != "0" ]]; then
  if command -v claude >/dev/null 2>&1; then
    echo "==> Registering MCP server with Claude Code"
    # HTTP transport talking to the local server; the app must be running.
    claude mcp add --transport http mac-transformer http://127.0.0.1:8771/mcp || \
      echo "note: MCP registration failed (already registered?); skipping"
  else
    echo "note: Claude Code CLI not found — skipping MCP registration"
  fi
fi

echo
echo "Install complete."
echo "  Launch:               open $APP_BUNDLE"
echo "  REST/MCP:             http://127.0.0.1:8771"
echo "  Finder Open With:     right-click a supported document -> Open With -> Mac Transformer"
echo "  Finder Quick Action:  right-click a file in Finder -> Quick Actions -> Send to Mac Transformer"
echo "  MCP tools in Claude:  restart Claude Code, then look for mcp__mac-transformer__*"
echo
echo "Tip: right-click a .adoc / .docx / .pptx / .pdf / .md file in Finder to see 'Open With → Mac Transformer'."
