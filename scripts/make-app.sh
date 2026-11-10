#!/bin/sh
# Bundle the window into Exodrive.app and sign it.
#
# Usage: scripts/make-app.sh <version> [output-dir]
#
# SIGN_IDENTITY selects the codesign identity, for example
# "Developer ID Application: Name (TEAMID)". Without it, the bundle gets an
# ad-hoc signature. An ad-hoc bundle runs on the machine that built it.
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

(cd "$ROOT" && go build -trimpath -ldflags "-s -w" -o "$APP/Contents/MacOS/Exodrive" ./cmd/exodrive-gui)

cp "$ROOT/README.md" "$ROOT/README.zh.md" "$ROOT/LICENSE" "$APP/Contents/Resources/"
sed "s/__VERSION__/$VERSION/g" "$ROOT/packaging/Info.plist.in" > "$APP/Contents/Info.plist"

codesign --force --sign "$IDENTITY" "$APP"
codesign --verify --strict "$APP"

echo "make-app: $APP (version $VERSION, identity: $IDENTITY)"
