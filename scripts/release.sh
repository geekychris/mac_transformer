#!/usr/bin/env bash
# release.sh — cut a signed, notarized, stapled DMG of Mac Transformer.
#
# Usage:
#   ./scripts/release.sh 0.1.0                       # normal release
#   ./scripts/release.sh 0.1.0 --skip-notarize       # local test
#   ./scripts/release.sh 0.1.0 --gh-release          # also create GitHub Release
#
# Prereqs (one-time):
#   ./scripts/setup-signing.sh
#
# Output:
#   dist/mac-transformer-<version>.dmg   — signed + stapled DMG
#   dist/mac-transformer-<version>.sha256 — sha256 of the DMG (paste into Cask)

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "usage: $0 VERSION [--skip-notarize] [--gh-release]" >&2
    exit 2
fi

VERSION="$1"
shift
SKIP_NOTARIZE=0
GH_RELEASE=0
for arg in "$@"; do
    case "$arg" in
        --skip-notarize) SKIP_NOTARIZE=1 ;;
        --gh-release)    GH_RELEASE=1 ;;
        *) echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Load signing config saved by setup-signing.sh
WAILS_DEFAULTS="$HOME/.config/wails/defaults.yaml"
if [[ ! -f "$WAILS_DEFAULTS" ]]; then
    echo "error: $WAILS_DEFAULTS not found. Run ./scripts/setup-signing.sh first." >&2
    exit 1
fi
SIGN_IDENTITY=$(awk -F': *' '/^sign_identity:/{gsub(/^"|"$/, "", $2); print $2}' "$WAILS_DEFAULTS")
KEYCHAIN_PROFILE=$(awk -F': *' '/^keychain_profile:/{gsub(/^"|"$/, "", $2); print $2}' "$WAILS_DEFAULTS")

if [[ -z "$SIGN_IDENTITY" ]]; then
    echo "error: no sign_identity in $WAILS_DEFAULTS" >&2
    exit 1
fi

echo "==> Releasing Mac Transformer $VERSION"
echo "  Signing identity: $SIGN_IDENTITY"
echo "  Notary profile:   $KEYCHAIN_PROFILE"
echo

# --- Update version strings in Info.plist ---
# The .plist gets baked into the .app during wails3 package.
plist=build/darwin/Info.plist
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $VERSION" "$plist" || true
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" "$plist" || true

# --- Build the .app ---
export PATH="$PATH:$HOME/go/bin"
if ! command -v wails3 >/dev/null 2>&1; then
    echo "error: wails3 CLI not found. Run: go install github.com/wailsapp/wails/v3/cmd/wails3@latest" >&2
    exit 1
fi

echo "==> Generating Wails bindings"
wails3 generate bindings >/dev/null

echo "==> Building .app bundle"
rm -rf bin/mac-transformer.app bin/mac-transformer
wails3 package >/dev/null

APP="bin/mac-transformer.app"
if [[ ! -d "$APP" ]]; then
    echo "error: $APP not created by wails3 package" >&2
    exit 1
fi

# --- Sign with Hardened Runtime + our real Developer ID ---
# The `wails3 package` step ran an ad-hoc signature; replace it.
echo "==> Signing with Developer ID + Hardened Runtime"
codesign --force --deep --options runtime \
    --entitlements build/darwin/entitlements.plist \
    --sign "$SIGN_IDENTITY" \
    --timestamp \
    "$APP"

codesign --verify --deep --strict --verbose=2 "$APP"

# --- Assemble DMG ---
DIST="dist"
mkdir -p "$DIST"
DMG="$DIST/mac-transformer-$VERSION.dmg"
rm -f "$DMG"

echo "==> Building DMG"
STAGING=$(mktemp -d)
trap 'rm -rf "$STAGING"' EXIT
cp -R "$APP" "$STAGING/Mac Transformer.app"
ln -s /Applications "$STAGING/Applications"

# Optional: put the DMG background image if present. Kept simple otherwise.
hdiutil create \
    -volname "Mac Transformer $VERSION" \
    -srcfolder "$STAGING" \
    -ov -format UDZO \
    "$DMG" > /dev/null

# Sign the DMG itself so Gatekeeper's first-launch flow is clean.
codesign --force --sign "$SIGN_IDENTITY" --timestamp "$DMG"

if [[ "$SKIP_NOTARIZE" == "1" ]]; then
    echo "==> Skipping notarization (--skip-notarize)"
else
    echo "==> Submitting to Apple Notary Service (this takes ~1–5 min)"
    xcrun notarytool submit "$DMG" \
        --keychain-profile "$KEYCHAIN_PROFILE" \
        --wait

    echo "==> Stapling notarization ticket"
    xcrun stapler staple "$DMG"
    xcrun stapler validate "$DMG"
fi

SHA=$(shasum -a 256 "$DMG" | awk '{print $1}')
echo "$SHA  $(basename "$DMG")" > "$DMG.sha256"

echo
echo "==> Release ready:"
echo "  DMG:    $DMG"
echo "  Bytes:  $(stat -f %z "$DMG")"
echo "  SHA256: $SHA"
echo
echo "Cask formula update:"
echo "  version \"$VERSION\""
echo "  sha256 \"$SHA\""
echo "  url \"https://github.com/geekychris/mac_transformer/releases/download/v$VERSION/$(basename "$DMG")\""

if [[ "$GH_RELEASE" == "1" ]]; then
    echo
    echo "==> Creating GitHub Release v$VERSION"
    gh release create "v$VERSION" "$DMG" "$DMG.sha256" \
        --title "Mac Transformer $VERSION" \
        --notes "## Install

\`\`\`sh
brew install --cask geekychris/mac-transformer/mac-transformer
\`\`\`

Or download and open the DMG directly."

    # Auto-update the Cask formula in the tap repo.
    TAP_DIR="${MT_TAP_DIR:-$HOME/.mac-transformer/homebrew-mac-transformer}"
    if [[ ! -d "$TAP_DIR/.git" ]]; then
        echo "==> Cloning tap into $TAP_DIR"
        git clone git@github.com:geekychris/homebrew-mac-transformer.git "$TAP_DIR" 2>&1 | tail -3 \
            || git clone https://github.com/geekychris/homebrew-mac-transformer.git "$TAP_DIR"
    else
        (cd "$TAP_DIR" && git pull --ff-only) 2>&1 | tail -3
    fi

    CASK="$TAP_DIR/Casks/mac-transformer.rb"
    if [[ -f "$CASK" ]]; then
        echo "==> Updating $CASK to $VERSION ($SHA)"
        sed -i.bak -E "s|^  version \"[^\"]+\"|  version \"$VERSION\"|" "$CASK"
        sed -i.bak -E "s|^  sha256 \"[a-f0-9]+\"|  sha256 \"$SHA\"|" "$CASK"
        rm -f "$CASK.bak"
        (cd "$TAP_DIR" &&
            git add Casks/mac-transformer.rb &&
            git commit -m "mac-transformer $VERSION" &&
            git push) 2>&1 | tail -3
    else
        echo "warning: $CASK not found — skipping tap update. Update it manually." >&2
    fi
fi
