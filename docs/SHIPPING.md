# Shipping a release

Everything needed to sign, notarize, and distribute Mac Transformer via GitHub Releases + Homebrew.

## Distribution model

Mac Transformer is **not** on the Mac App Store — the app's plugin system (spawning arbitrary converters), REST server, CLI, and Automator workflow all fight the App Store sandbox. Instead we ship as a **signed, notarized DMG** distributed via:

1. **GitHub Releases** — direct download, human users
2. **Homebrew Cask** — `brew install --cask geekychris/mac-transformer/mac-transformer`

Both are backed by the same signed DMG artifact.

## One-time setup

You need an Apple Developer Program membership ($99/yr). Then, in this order:

### 1. Install a Developer ID Application certificate

- Open Xcode → **Settings** → **Accounts**
- Add your Apple ID (if not already there)
- Select your team → **Manage Certificates**
- Click **+** → **Developer ID Application**

Xcode installs the certificate into your login keychain. Verify:

```sh
security find-identity -v -p codesigning | grep "Developer ID Application"
```

You should see a line like `1) ABCDEF… "Developer ID Application: Your Name (TEAMID)"`.

### 2. Create a notarytool app-specific password

- Go to https://appleid.apple.com → **Sign-In and Security** → **App-Specific Passwords**
- Generate a new password, labeled e.g. `notarytool`
- **Keep the popup open** — you'll paste it into the setup script in a moment.

### 3. Run the setup script

```sh
./scripts/setup-signing.sh
```

It:

- Detects your Developer ID cert(s), asks which to use
- Extracts your Team ID from the identity string
- Asks for your Apple ID + the app-specific password from step 2
- Stores the notarytool credentials in your login keychain as profile `mac-transformer-notary`
- Writes `~/.config/wails/defaults.yaml` with the signing identity + profile name

Idempotent — safe to re-run to rotate credentials.

## Cutting a release

```sh
./scripts/release.sh 0.1.0 --gh-release
```

Steps:

1. Bumps `CFBundleVersion` + `CFBundleShortVersionString` in `build/darwin/Info.plist`
2. Runs `wails3 generate bindings`
3. Runs `wails3 package` (builds frontend + Go binary + assembles .app)
4. Re-signs with your **Developer ID** + **Hardened Runtime** + `build/darwin/entitlements.plist` (replaces the ad-hoc signature)
5. Verifies the signature (`codesign --verify --deep --strict`)
6. Assembles the DMG (staging dir with `Mac Transformer.app` + `/Applications` symlink)
7. Signs the DMG
8. Submits to Apple's Notary Service (`xcrun notarytool submit --wait` — ~1–5 min)
9. Staples the notarization ticket (`xcrun stapler staple`) so Gatekeeper works offline
10. Emits `dist/mac-transformer-<version>.dmg` + `.sha256`
11. (with `--gh-release`) Creates a GitHub Release and uploads the DMG
12. (with `--gh-release`) Auto-updates the [Homebrew tap](https://github.com/geekychris/homebrew-mac-transformer) — pulls, rewrites version + sha256 in `Casks/mac-transformer.rb`, commits, pushes

Flags:

- `--skip-notarize` — sign locally but skip the Apple round-trip; useful for testing the build+sign path
- `--gh-release` — after building the DMG, cut a GitHub Release and update the Cask

Environment overrides:

- `MT_TAP_DIR` — where to clone the tap for auto-updating the Cask (default: `~/.mac-transformer/homebrew-mac-transformer`)

## Before your first release: bundle ID

The current `build/darwin/Info.plist` uses `CFBundleIdentifier = com.chris.mac-transformer`. This is a placeholder — before you cut a real release, change it to a reverse-DNS you own. Apple recommends `com.<yourdomain>.mac-transformer` or `com.<yourgithubhandle>.mac-transformer`. The bundle id sticks with the app forever (uninstalling won't clear it from Launch Services caches), so pick well:

```sh
# In build/darwin/Info.plist AND build/darwin/Info.dev.plist:
/usr/libexec/PlistBuddy -c "Set :CFBundleIdentifier com.yourhandle.mac-transformer" build/darwin/Info.plist
/usr/libexec/PlistBuddy -c "Set :CFBundleIdentifier com.yourhandle.mac-transformer.dev" build/darwin/Info.dev.plist
```

Also grep the codebase for any references to the old id (`main.go` and any AppleScripts / plists that check bundle_id) and update those too:

```sh
grep -rn "com.chris.mac-transformer" .
```

## What the entitlements grant

`build/darwin/entitlements.plist` declares only what Hardened Runtime needs:

- `com.apple.security.automation.apple-events` — `osascript` control of Books, Music, Safari, Word, PowerPoint. Info.plist has the matching `NSAppleEventsUsageDescription` so macOS's TCC prompts the user on first use.
- `com.apple.security.cs.disable-library-validation` — we spawn subprocesses (pandoc, soffice, ffmpeg, swift, xattr, curl, osascript) that aren't signed by our team. Without this, Hardened Runtime blocks them.
- `com.apple.security.cs.allow-unsigned-executable-memory` + `com.apple.security.cs.allow-jit` — some pdfcpu and Go runtime paths JIT-compile; the flags avoid a runtime kill.

We are **not sandboxed**, so no `com.apple.security.files.*` or `com.apple.security.network.*` entitlements are needed. macOS still prompts the user for TCC access to Documents/Desktop/Downloads on first use, thanks to the `NSDocumentsFolderUsageDescription`/etc. keys in Info.plist.

## Verification checklist

After each release, before announcing:

```sh
# 1. The .app inside the DMG passes strict verification
hdiutil attach dist/mac-transformer-0.1.0.dmg -mountpoint /tmp/mt-verify
codesign --verify --deep --strict --verbose=2 "/tmp/mt-verify/Mac Transformer.app"
spctl --assess --type execute --verbose "/tmp/mt-verify/Mac Transformer.app"
# expect: accepted; source=Notarized Developer ID

# 2. The stapled ticket is present
xcrun stapler validate dist/mac-transformer-0.1.0.dmg
# expect: worked!

hdiutil detach /tmp/mt-verify

# 3. Homebrew Cask installs cleanly on a fresh Mac (or fresh brew prefix)
brew tap geekychris/mac-transformer
brew install --cask mac-transformer
# expect: no Gatekeeper prompt beyond the standard "downloaded from internet" note
```

## Distribution mechanics

- Users who install via Homebrew: `brew install --cask geekychris/mac-transformer/mac-transformer`
- Users who download the DMG directly: drag `Mac Transformer.app` to `/Applications`
- Both routes trigger `lsregister -f` on first launch (the Cask does it in `postflight`; the DMG relies on the `install.sh` bundled script if the user runs it)

The user should see **no "unidentified developer" dialog** — a notarized + stapled DMG passes Gatekeeper cleanly.

## Rotating certificates

Developer ID certs expire every 5 years. Renewal:

1. Xcode → Settings → Accounts → Manage Certificates → **+** → Developer ID Application (creates a new one)
2. Re-run `./scripts/setup-signing.sh` — pick the new identity
3. Next `./scripts/release.sh` uses it

Old signatures on already-shipped DMGs remain valid; only NEW builds need to switch.
