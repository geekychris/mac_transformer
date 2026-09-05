#!/usr/bin/env bash
# setup-signing.sh — one-time configuration for release signing + notarization.
#
# Discovers the Developer ID Application certificate in your login keychain,
# prompts for a notarytool app-specific password, stores everything Wails
# and `notarytool` need. Idempotent — safe to re-run.
#
# Prereqs (do these ONCE, in this order, then run this script):
#   1. Enroll in the Apple Developer Program at developer.apple.com.
#   2. In Xcode → Settings → Accounts → Manage Certificates → "+", create
#      a "Developer ID Application" certificate. Xcode installs it into
#      your login keychain automatically.
#   3. At appleid.apple.com → Sign-In & Security → App-Specific Passwords,
#      generate a password labeled e.g. "notarytool". Keep the window open
#      when you run this script; you'll paste it in when asked.

set -euo pipefail

echo "==> Mac Transformer release-signing setup"
echo

# --- 1. Find Developer ID Application cert(s) ---
identities=$(security find-identity -v -p codesigning | grep "Developer ID Application" || true)
if [[ -z "$identities" ]]; then
    cat <<'MSG' >&2
error: no "Developer ID Application" identity found in your login keychain.

Fix:
  1. Enroll at developer.apple.com if you haven't already.
  2. Open Xcode → Settings → Accounts → your Apple ID → Manage Certificates.
  3. Click "+" and pick "Developer ID Application".
  4. Re-run this script.
MSG
    exit 1
fi
echo "Found signing identities:"
echo "$identities"
echo

# Take the first one by default; the user can override.
default_identity=$(echo "$identities" | head -1 | sed -E 's/^[[:space:]]*[0-9]+\)[[:space:]]+[[:xdigit:]]+[[:space:]]+"([^"]+)".*/\1/')
read -r -p "Signing identity to use [$default_identity]: " chosen_identity
identity="${chosen_identity:-$default_identity}"
echo

# --- 2. Team ID (used later for notarytool) ---
# The team ID is the 10-char code inside the parens of the identity string.
team_id=$(echo "$identity" | sed -nE 's/.*\(([A-Z0-9]{10})\)$/\1/p')
if [[ -z "$team_id" ]]; then
    read -r -p "Couldn't auto-detect Team ID; enter it (10-char code): " team_id
fi
echo "Team ID: $team_id"

# --- 3. Apple ID + app-specific password for notarytool ---
read -r -p "Apple ID (the email you use for developer.apple.com): " apple_id
echo
echo "Now paste your notarytool app-specific password."
echo "  (Generate at appleid.apple.com → Sign-In and Security → App-Specific Passwords.)"
read -r -s -p "App-specific password: " apple_pw
echo

# Store as a notarytool keychain profile named "mac-transformer-notary".
# This is the profile name our release script will pass with --keychain-profile.
xcrun notarytool store-credentials mac-transformer-notary \
    --apple-id "$apple_id" \
    --team-id "$team_id" \
    --password "$apple_pw" \
    2>&1 | tail -3

# --- 4. Persist Wails signing defaults ---
mkdir -p "$HOME/.config/wails"
config="$HOME/.config/wails/defaults.yaml"
if [[ -f "$config" ]]; then
    # Preserve any unrelated keys the file already has.
    tmp="$(mktemp)"
    grep -Ev '^\s*(sign_identity|apple_id|team_id|keychain_profile):' "$config" > "$tmp" || true
    mv "$tmp" "$config"
fi
cat <<CFG >> "$config"
sign_identity: "$identity"
apple_id: "$apple_id"
team_id: "$team_id"
keychain_profile: mac-transformer-notary
CFG

echo
echo "Signing setup complete."
echo "  Identity:          $identity"
echo "  Team ID:           $team_id"
echo "  Notary profile:    mac-transformer-notary"
echo "  Wails config:      $config"
echo
echo "You can now cut a release: ./scripts/release.sh 0.1.0"
