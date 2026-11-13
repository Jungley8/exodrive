#!/bin/sh
# Install Exodrive.
#
#   curl -fsSL https://jungley8.github.io/exodrive/install.sh | sh
#
# The script reads the newest release, downloads the archive for this machine,
# checks it against SHA256SUMS, and installs the command line tool and, on
# macOS, the app bundle. A failed check stops the install.
#
# Options:
#   --cli              Install the command line tool only
#   --app              Install the app bundle only (macOS)
#   --version <tag>    Install one release, for example v0.7.0
#   --dir <folder>     Install into this folder
#   --help             Print this text
set -eu

REPO="Jungley8/exodrive"
SITE="https://jungley8.github.io/exodrive"

VERSION=""
DIR=""
WHAT="all"

say() { printf '%s\n' "$*"; }
fail() {
	printf 'install: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Install Exodrive.

  curl -fsSL https://jungley8.github.io/exodrive/install.sh | sh

The script reads the newest release, downloads the archive for this machine,
checks it against SHA256SUMS, and installs the command line tool and, on
macOS, the app bundle. A failed check stops the install.

Options:
  --cli              Install the command line tool only
  --app              Install the app bundle only (macOS)
  --version <tag>    Install one release, for example v0.7.0
  --dir <folder>     Install into this folder
  --help             Print this text
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--cli) WHAT="cli" ;;
	--app) WHAT="app" ;;
	--version)
		[ $# -ge 2 ] || fail "--version needs a tag, for example v0.7.0"
		VERSION="$2"
		shift
		;;
	--dir)
		[ $# -ge 2 ] || fail "--dir needs a folder"
		DIR="$2"
		shift
		;;
	--help | -h)
		usage
		exit 0
		;;
	*) fail "unknown option '$1'. Run with --help." ;;
	esac
	shift
done

case "$(uname -s)" in
Darwin) OS="darwin" ;;
Linux) OS="linux" ;;
*) fail "Exodrive installs on macOS and Linux. This system is not supported." ;;
esac

case "$(uname -m)" in
arm64 | aarch64) ARCH="arm64" ;;
x86_64 | amd64) ARCH="amd64" ;;
*) fail "No Exodrive build exists for this processor." ;;
esac

if [ "$OS" != "darwin" ] && [ "$WHAT" = "app" ]; then
	fail "The app bundle ships for macOS only. Use --cli on Linux."
fi
if [ "$OS" = "linux" ]; then
	WHAT="cli"
fi

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL --retry 2 -o "$2" "$1"; }
	latest() { curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q -O "$2" "$1"; }
	latest() { wget --max-redirect=0 -O /dev/null -S "https://github.com/$REPO/releases/latest" 2>&1 | sed -n 's/^ *Location: //p' | head -n 1; }
else
	fail "curl or wget is required."
fi

if [ -z "$VERSION" ]; then
	URL=$(latest) || fail "Cannot read the release list."
	VERSION=${URL##*/}
	[ -n "$VERSION" ] || fail "Cannot read the release list."
fi
case "$VERSION" in
v*) ;;
*) VERSION="v$VERSION" ;;
esac

BASE="https://github.com/$REPO/releases/download/$VERSION"
BIN_TAR="exodrive_${VERSION}_${OS}_${ARCH}.tar.gz"
APP_ZIP="Exodrive_${VERSION}_darwin_${ARCH}.app.zip"

TMP=$(mktemp -d "${TMPDIR:-/tmp}/exodrive.XXXXXX") || fail "Cannot make a temporary folder."
trap 'rm -rf "$TMP"' EXIT INT TERM

install_cli() {
	say "Downloading $BIN_TAR"
	fetch "$BASE/$BIN_TAR" "$TMP/$BIN_TAR" || fail "Cannot download $BIN_TAR."
	verify "$BIN_TAR"

	tar xzf "$TMP/$BIN_TAR" -C "$TMP" || fail "Cannot unpack $BIN_TAR."
	src="$TMP/exodrive_${OS}_${ARCH}/exodrive"
	[ -f "$src" ] || fail "The archive holds no exodrive."

	dest="${DIR:-$HOME/.local/bin}"
	mkdir -p "$dest" || fail "Cannot make $dest."
	install -m 0755 "$src" "$dest/exodrive" || fail "Cannot write $dest/exodrive."
	say "Installed $dest/exodrive"
	case ":$PATH:" in
	*":$dest:"*) ;;
	*) say "Add $dest to PATH, then run: exodrive check" ;;
	esac
}

install_app() {
	say "Downloading $APP_ZIP"
	fetch "$BASE/$APP_ZIP" "$TMP/$APP_ZIP" || fail "Cannot download $APP_ZIP."
	verify "$APP_ZIP"

	unzip -q "$TMP/$APP_ZIP" -d "$TMP" || fail "Cannot unpack $APP_ZIP."
	[ -d "$TMP/Exodrive.app" ] || fail "The archive holds no Exodrive.app."

	dest="${DIR:-$HOME/Applications}"
	mkdir -p "$dest" || fail "Cannot make $dest."
	rm -rf "$dest/Exodrive.app"
	cp -R "$TMP/Exodrive.app" "$dest/Exodrive.app" || fail "Cannot write $dest/Exodrive.app."
	say "Installed $dest/Exodrive.app"
	say "A download from a browser asks for confirmation on the first start. Right-click the app, then choose Open."
}

# verify reads the one line that names an asset, so a missing neighbour in
# SHA256SUMS never fails this install. Releases before v0.8.0 write "./name".
verify() {
	asset="$1"
	fetch "$BASE/SHA256SUMS" "$TMP/SHA256SUMS" || fail "Cannot download SHA256SUMS."
	line=$(awk -v a="$asset" '$2 == a || $2 == "./" a' "$TMP/SHA256SUMS")
	[ -n "$line" ] || fail "SHA256SUMS holds no line for $asset."
	printf '%s\n' "$line" >"$TMP/ONE"
	if command -v shasum >/dev/null 2>&1; then
		(cd "$TMP" && shasum -a 256 -c ONE >/dev/null) || fail "$asset does not match SHA256SUMS. Nothing was installed."
	elif command -v sha256sum >/dev/null 2>&1; then
		(cd "$TMP" && sha256sum -c ONE >/dev/null) || fail "$asset does not match SHA256SUMS. Nothing was installed."
	else
		fail "No SHA-256 tool found. Install shasum or sha256sum."
	fi
	say "Checked $asset against SHA256SUMS"
}

say "Exodrive $VERSION for $OS/$ARCH"
case "$WHAT" in
all)
	install_cli
	install_app
	;;
cli) install_cli ;;
app) install_app ;;
esac
say "Done. Read the documentation at $SITE/docs/"
