#!/bin/sh
# Submit Exodrive.app to Apple for notarization, then staple the ticket.
#
# Usage: scripts/notarize.sh [app-path]
#
# Notarization needs a Developer ID signature and a stored notarytool profile.
# Create the profile once:
#
#   xcrun notarytool store-credentials exodrive-notary \
#     --apple-id you@example.com --team-id TEAMID --password APP_PASSWORD
#
# Then:
#
#   SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" make app
#   NOTARY_PROFILE=exodrive-notary make notarize
#
# The script stops on the first failure. A bundle without a ticket keeps the
# Gatekeeper prompt, so it must not ship as a notarized build.
set -eu

APP="${1:-bin/Exodrive.app}"
NOTARY_PROFILE="${NOTARY_PROFILE:-}"

if [ "$(uname -s)" != "Darwin" ]; then
	echo "notarize: notarization needs macOS" >&2
	exit 1
fi

if [ ! -d "$APP" ]; then
	echo "notarize: $APP is missing. Run 'make app' first." >&2
	exit 1
fi

if [ -z "$NOTARY_PROFILE" ]; then
	echo "notarize: NOTARY_PROFILE is not set. Create a profile with 'xcrun notarytool store-credentials', then set NOTARY_PROFILE." >&2
	exit 1
fi

# An ad-hoc signature cannot be notarized. Check the identity before the upload,
# so the failure names the cause.
if ! codesign -dv --verbose=2 "$APP" 2>&1 | grep -q "Authority=Developer ID Application"; then
	echo "notarize: $APP is not signed with a Developer ID. Set SIGN_IDENTITY, then run 'make app' again." >&2
	exit 1
fi

echo "notarize: uploading $APP"
ARCHIVE="$(mktemp -t exodrive-notarize).zip"
trap 'rm -f "$ARCHIVE"' EXIT
ditto -c -k --keepParent "$APP" "$ARCHIVE"

if ! xcrun notarytool submit "$ARCHIVE" --keychain-profile "$NOTARY_PROFILE" --wait; then
	echo "notarize: Apple rejected the upload or the bundle. Read the log with 'xcrun notarytool log <id> --keychain-profile $NOTARY_PROFILE'." >&2
	exit 1
fi

xcrun stapler staple "$APP"
xcrun stapler validate "$APP"

# A stapled bundle passes the Gatekeeper assessment without a network lookup.
if ! spctl --assess --type execute -vv "$APP"; then
	echo "notarize: Gatekeeper still rejects $APP." >&2
	exit 1
fi

echo "notarize: $APP is signed, notarized, and stapled"
