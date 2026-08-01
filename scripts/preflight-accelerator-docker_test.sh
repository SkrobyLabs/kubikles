#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
make_bin=$(command -v make)
trap 'rm -rf "$tmp"' EXIT INT TERM

fail() { echo "preflight regression failed: $*" >&2; exit 1; }
run_case() {
	name=$1
	mode=$2
	needle=$3
	shim="$tmp/$name"
	mkdir "$shim"
	case "$mode" in
	missing) ;;
	info|buildx)
		cat >"$shim/docker" <<'SH'
#!/bin/sh
echo "$*" >>"$DOCKER_RECORD"
case "$1${2:+ $2}" in
  info) [ "$DOCKER_MODE" = info ] && exit 1 ;;
  buildx\ version) [ "$DOCKER_MODE" = buildx ] && exit 1 ;;
esac
exit 0
SH
		chmod +x "$shim/docker"
		;;
	esac
	: >"$tmp/$name.record"
	path="$shim:/usr/bin:/bin"
	if [ "$mode" = missing ]; then path=$shim; fi
	if PATH="$path" DOCKER_RECORD="$tmp/$name.record" DOCKER_MODE="$mode" "$make_bin" -C "$root" -j test-accelerator-image >"$tmp/$name.out" 2>&1; then
		fail "$name unexpectedly passed"
	fi
	grep -F "$needle" "$tmp/$name.out" >/dev/null || fail "$name omitted actionable guidance"
	if grep -Eq 'buildx build|build ' "$tmp/$name.record"; then
		fail "$name invoked an image build after preflight failure"
	fi
}

run_case missing missing 'Docker is required: install Docker Engine with Buildx and start its daemon.'
run_case daemon info 'Docker daemon is unavailable: start Docker Engine and retry.'
run_case buildx buildx 'Docker Buildx is required: install/enable the Buildx plugin and retry.'
