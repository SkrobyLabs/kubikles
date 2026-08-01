#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

make_fixture() {
	dir=$1
	mkdir -p "$dir/assets/nested"
	printf '<!doctype html><title>fixture</title>\n' >"$dir/index.html"
	printf 'console.log("fixture")\n' >"$dir/assets/app.js"
	printf 'body { color: blue; }\n' >"$dir/assets/app.css"
	printf '<svg xmlns="http://www.w3.org/2000/svg"/>\n' >"$dir/assets/nested/icon.svg"
	printf '<p>remove original</p>\n' >"$dir/assets/nested/page.html"
	printf 'font fixture\n' >"$dir/assets/font.woff2"
	printf 'build stats\n' >"$dir/stats.html"
}

first="$tmp/short/dist"
second="$tmp/a-deliberately-different-parent-path/dist"
make_fixture "$first"
make_fixture "$second"
touch -t 200101010101 "$first/index.html" "$first/assets/app.js" "$first/assets/app.css" "$first/assets/nested/icon.svg" "$first/assets/nested/page.html"
touch -t 203712312359 "$second/index.html" "$second/assets/app.js" "$second/assets/app.css" "$second/assets/nested/icon.svg" "$second/assets/nested/page.html"

"$root/scripts/compress-dist.sh" "$first" >/dev/null
"$root/scripts/compress-dist.sh" "$second" >/dev/null

for relative in index.html.gz assets/app.js.gz assets/app.css.gz assets/nested/icon.svg.gz assets/nested/page.html.gz; do
	cmp "$first/$relative" "$second/$relative" >/dev/null || {
		echo "compress-dist fixture differs for $relative" >&2
		exit 1
	}
	set -- $(od -An -tu1 -N8 "$first/$relative")
	[ "$1" = 31 ] && [ "$2" = 139 ] && [ "$3" = 8 ] || {
		echo "invalid gzip header for $relative" >&2
		exit 1
	}
	[ "$4" = 0 ] || {
		echo "gzip optional filename/header flags are present for $relative" >&2
		exit 1
	}
	[ "$5" = 0 ] && [ "$6" = 0 ] && [ "$7" = 0 ] && [ "$8" = 0 ] || {
		echo "gzip mtime is nonzero for $relative" >&2
		exit 1
	}
done

[ -f "$first/index.html" ] && [ -f "$second/index.html" ] || {
	echo "index.html was not retained" >&2
	exit 1
}
for relative in assets/app.js assets/app.css assets/nested/icon.svg assets/nested/page.html; do
	[ ! -e "$first/$relative" ] && [ ! -e "$second/$relative" ] || {
		echo "compressible original was retained: $relative" >&2
		exit 1
	}
done
[ -f "$first/assets/font.woff2" ] && [ -f "$second/assets/font.woff2" ] || {
	echo "non-compressible original was removed" >&2
	exit 1
}
[ ! -e "$first/stats.html" ] && [ ! -e "$second/stats.html" ] || {
	echo "stats.html was retained" >&2
	exit 1
}

echo "compress-dist deterministic fixture passed"
