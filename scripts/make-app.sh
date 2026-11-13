#!/bin/sh
# Bundle the window into Exodrive.app and sign it.
#
# Usage: scripts/make-app.sh <version> [output-dir]
#
# SIGN_IDENTITY selects the codesign identity, for example
# "Developer ID Application: Name (TEAMID)". Without it, the bundle gets an
# ad-hoc signature. macOS asks for confirmation once before it opens an ad-hoc
# bundle. A Developer ID signature carries the hardened runtime and a secure
# timestamp, which notarization requires.
set -eu

VERSION="${1:?usage: make-app.sh <version> [output-dir]}"
OUT="${2:-bin}"
IDENTITY="${SIGN_IDENTITY:--}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ "$(uname -s)" != "Darwin" ]; then
	echo "make-app: an app bundle needs macOS" >&2
	exit 1
fi

mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
APP="$OUT/Exodrive.app"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

(cd "$ROOT" && go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$APP/Contents/MacOS/Exodrive" ./cmd/exodrive-gui)

cp "$ROOT/README.md" "$ROOT/README.zh.md" "$ROOT/LICENSE" "$ROOT/packaging/AppIcon.icns" "$APP/Contents/Resources/"
sed "s/__VERSION__/$VERSION/g" "$ROOT/packaging/Info.plist.in" > "$APP/Contents/Info.plist"

if [ "$IDENTITY" = "-" ]; then
	codesign --force --sign - "$APP"
else
	# Notarization needs the hardened runtime and a secure timestamp. An
	# ad-hoc identity can hold neither, so the flags apply here only.
	codesign --force --options runtime --timestamp --sign "$IDENTITY" "$APP"
fi
codesign --verify --strict "$APP"

echo "make-app: $APP (version $VERSION, identity: $IDENTITY)"
