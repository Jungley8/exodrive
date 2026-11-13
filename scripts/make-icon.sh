#!/bin/sh
# Build packaging/AppIcon.icns from packaging/icon.svg.
#
# Usage: scripts/make-icon.sh
#
# The SVG is the icon's source of truth. macOS renders it through Quick Look,
# scales that one image to every size an iconset holds, and packs the set.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SVG="$ROOT/packaging/icon.svg"
ICNS="$ROOT/packaging/AppIcon.icns"

if [ "$(uname -s)" != "Darwin" ]; then
	echo "make-icon: an icns needs macOS" >&2
	exit 1
fi
for tool in qlmanage sips iconutil; do
	command -v "$tool" >/dev/null || { echo "make-icon: $tool is missing" >&2; exit 1; }
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

qlmanage -t -s 1024 -o "$TMP" "$SVG" >/dev/null 2>&1 || true
BASE="$TMP/icon.svg.png"
if [ ! -f "$BASE" ]; then
	echo "make-icon: Quick Look did not render $SVG" >&2
	exit 1
fi

SET="$TMP/AppIcon.iconset"
mkdir -p "$SET"
for size in 16 32 128 256 512; do
	sips -z "$size" "$size" "$BASE" --out "$SET/icon_${size}x${size}.png" >/dev/null
	sips -z $((size * 2)) $((size * 2)) "$BASE" --out "$SET/icon_${size}x${size}@2x.png" >/dev/null
done

iconutil -c icns "$SET" -o "$ICNS"
echo "make-icon: $ICNS"
