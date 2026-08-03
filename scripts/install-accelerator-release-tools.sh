#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'accelerator release tools: %s\n' "$1" >&2; exit 1; }
[ "$#" -eq 1 ] || fail "usage: install-accelerator-release-tools.sh DESTINATION"
destination=$1
[ -n "$destination" ] || fail "destination is required"
mkdir -p "$destination"
chmod 700 "$destination"
tmp=$(mktemp -d "${RUNNER_TEMP:-/tmp}/accelerator-tools.XXXXXX")
chmod 700 "$tmp"
trap 'rm -rf "$tmp"' EXIT

fetch() {
  local url=$1 checksum=$2 archive=$3
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$url" -o "$tmp/$archive"
  printf '%s  %s\n' "$checksum" "$tmp/$archive" | sha256sum --check --status || fail "checksum verification failed for $archive"
}

fetch "https://get.helm.sh/helm-v3.21.3-linux-amd64.tar.gz" "15e041a93a590dce8100f39385cd98c84a765c9e36aeeb9e2dc6ff9e4769e2e0" helm.tar.gz
tar -xzf "$tmp/helm.tar.gz" -C "$tmp"
install -m 0755 "$tmp/linux-amd64/helm" "$destination/helm"

fetch "https://github.com/oras-project/oras/releases/download/v1.3.3/oras_1.3.3_linux_amd64.tar.gz" "9ce999f8d2de03fc03968b29d743077a58783e545e5eaa53917ca177352d0e59" oras.tar.gz
tar -xzf "$tmp/oras.tar.gz" -C "$tmp" oras
install -m 0755 "$tmp/oras" "$destination/oras"

fetch "https://github.com/cli/cli/releases/download/v2.97.0/gh_2.97.0_linux_amd64.tar.gz" "a2c9b8497e1f85b1ad0dfcb78b5a622e098801b8e461e459e88e1ee12f018112" gh.tar.gz
tar -xzf "$tmp/gh.tar.gz" -C "$tmp"
install -m 0755 "$tmp/gh_2.97.0_linux_amd64/bin/gh" "$destination/gh"

fetch "https://github.com/anchore/syft/releases/download/v1.44.0/syft_1.44.0_linux_amd64.tar.gz" "0e91737aee2b5baf1d255b959630194a302335d848ff97bb07921eb6205b5f5a" syft.tar.gz
tar -xzf "$tmp/syft.tar.gz" -C "$tmp" syft
install -m 0755 "$tmp/syft" "$destination/syft"

fetch "https://github.com/aquasecurity/trivy/releases/download/v0.70.0/trivy_0.70.0_Linux-64bit.tar.gz" "8b4376d5d6befe5c24d503f10ff136d9e0c49f9127a4279fd110b727929a5aa9" trivy.tar.gz
tar -xzf "$tmp/trivy.tar.gz" -C "$tmp" trivy
install -m 0755 "$tmp/trivy" "$destination/trivy"

[ "$("$destination/helm" version --short)" = 'v3.21.3+g1ad6e68' ] || fail "installed Helm build differs from canonical v3.21.3"
[ "$("$destination/oras" version | sed -n 's/^Version:[[:space:]]*//p')" = '1.3.3' ] || fail "installed ORAS version differs from 1.3.3"
[ "$("$destination/oras" version | sed -n 's/^Git commit:[[:space:]]*//p')" = '210747c29c1d38732b3194878dfd8b5a6b9ad7eb' ] || fail "installed ORAS build differs from canonical v1.3.3"
[ "$("$destination/gh" --version | sed -n '1p')" = 'gh version 2.97.0 (2026-07-31)' ] || fail "installed GitHub CLI build differs from canonical v2.97.0"
[ "$("$destination/syft" version -o json | jq -r .version)" = '1.44.0' ] || fail "installed Syft build differs from canonical v1.44.0"
[ "$("$destination/trivy" --version | sed -n 's/^Version: //p')" = '0.70.0' ] || fail "installed Trivy build differs from canonical v0.70.0"
