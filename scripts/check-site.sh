#!/bin/sh
# Check the pages under site/ before GitHub Pages serves them.
#
# Usage: scripts/check-site.sh
#
# The pages have no build step, so this script is the build: every page needs a
# language, a title, and links that resolve. A page in one language needs its
# twin in the other.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SITE="$ROOT/site"
fail=0

note() {
	echo "check-site: $1" >&2
	fail=1
}

pages="$(find "$SITE" -name '*.html' | sort)"
[ -n "$pages" ] || { note "site/ holds no page"; exit 1; }

for page in $pages; do
	rel="${page#"$SITE"/}"
	dir="$(dirname "$page")"

	grep -q '<html lang="' "$page" || note "$rel has no lang"
	grep -q '<title>' "$page" || note "$rel has no title"
	grep -q 'name="viewport"' "$page" || note "$rel has no viewport"

	# Every relative link resolves to a file or a folder with a page.
	for href in $(sed -n 's/.*href="\([^"]*\)".*/\1/p' "$page"); do
		case "$href" in
		http* | mailto:* | \#*) continue ;;
		esac
		target="${href%%#*}"
		[ -n "$target" ] || continue
		if [ ! -e "$dir/$target" ]; then
			note "$rel links to $href, which is not there"
		fi
	done
done

# Each page needs the same page in the other language.
for page in $pages; do
	rel="${page#"$SITE"/}"
	case "$rel" in
	zh/*) twin="${rel#zh/}"; twin="$SITE/$twin" ;;
	*) twin="$SITE/zh/$rel" ;;
	esac
	[ -e "$twin" ] || note "$rel has no page in the other language"
done

exit "$fail"
