package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectedCollisionHelperPreservesErrexit(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
marker="$2/reached"
fail_halfway() { false; printf reached > "$marker"; }
expect_publication_failure collision "$2/collision.log" fail_halfway
[ ! -e "$marker" ]
[ ! -s "$2/collision.log" ]
`, "collision-test", publisher, tmp)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("expected-collision helper lost strict status semantics: %v (%s)", err, output)
	}
}

func TestHelmDigestPullMakesArchivePrivate(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
helm() {
  local destination=''
  while [ "$#" -gt 0 ]; do
    if [ "$1" = --destination ]; then destination=$2; shift 2; else shift; fi
  done
  [ -n "$destination" ]
  printf chart > "$destination/pulled.tgz"
  chmod 0644 "$destination/pulled.tgz"
}
mkdir -m 0700 "$2/pull"
HELM_REGISTRY_CONFIG="$2/registry.json"
helm_pull_digest example.invalid/chart sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa false "$2/pull"
[ -z "$(find "$2/pull" -maxdepth 1 -type f -name '*.tgz' ! -perm 600 -print -quit)" ]
`, "helm-pull-test", publisher, tmp)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("digest-pulled chart archive was not made private: %v (%s)", err, output)
	}
}

func TestPublisherScopesGitHubTokenToGitHubCLI(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
GH_TOKEN=hostile-token-do-not-inherit
capture_gh_token
[ -z "${GH_TOKEN+x}" ]
non_github_child() { [ -z "${GH_TOKEN+x}" ]; }
gh() { [ "$GH_TOKEN" = hostile-token-do-not-inherit ]; printf scoped; }
non_github_child
[ "$(github_cli api test)" = scoped ]
`, "token-scope-test", publisher)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("GitHub token scope is not narrow: %v (%s)", err, output)
	}
}

func TestVerifierUsesBareChartRepositoryForBlobAndHelmPull(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
test_root=$2
ORAS_CONFIG="$2/oras.json"
HELM_REGISTRY_CONFIG="$2/helm.json"
mkdir -m 0700 "$2/pull"
oras() { printf '%s\n' "$*" >> "$test_root/oras-args"; printf blob > "$test_root/config.json"; }
helm() { printf '%s\n' "$*" >> "$test_root/helm-args"; printf chart > "$test_root/pull/chart.tgz"; }
digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
fetch_registry_blob example.invalid/helm/chart "$digest" false "$2/config.json"
helm_pull_digest example.invalid/helm/chart "$digest" false "$2/pull"
[ "$(grep -o '@' "$2/oras-args" | wc -l | tr -d ' ')" -eq 1 ]
[ "$(grep -o '@' "$2/helm-args" | wc -l | tr -d ' ')" -eq 1 ]
! grep -q '@.*@' "$2/oras-args" "$2/helm-args"
`, "chart-repository-test", publisher, t.TempDir())
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("chart repository was not normalized before digest fetches: %v (%s)", err, output)
	}
}

func TestRemoteTagMovementPreventsEveryRegistryWrite(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
PUBLICATION_AUTHORITY=github
expected=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
remote_tag_commit() { printf '%s\n' bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb; }
oras_copy_from_layout() { : > "$2/wrote"; }
for artifact in image chart; do
  mkdir -p "$2/$artifact"
  if (write_registry_artifact v1.2.3 "$expected" layout:v1.2.3 "$2/$artifact" false); then
    exit 1
  fi
  [ ! -e "$2/$artifact/wrote" ]
done
`, "tag-race-test", publisher, t.TempDir())
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("remote tag movement did not preserve zero-write behavior: %v (%s)", err, output)
	}
}

func TestReleaseMetadataMustBeFinalAndExact(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
tab=$'\t'
validate_release_metadata v1.2.3 "v1.2.3${tab}v1.2.3${tab}false${tab}false"
validate_release_metadata v1.2.3-alpha.1 "v1.2.3-alpha.1${tab}v1.2.3-alpha.1${tab}false${tab}true"
for metadata in \
  "v1.2.4${tab}v1.2.3${tab}false${tab}false" \
  "v1.2.3${tab}different${tab}false${tab}false" \
  "v1.2.3${tab}v1.2.3${tab}true${tab}false" \
  "v1.2.3${tab}v1.2.3${tab}false${tab}true"; do
  if (validate_release_metadata v1.2.3 "$metadata"); then exit 1; fi
done
`, "release-metadata-test", publisher)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("GitHub Release metadata contract was not exact: %v (%s)", err, output)
	}
}

func TestHostileCredentialsUsePrivateInputsAndLeakScanner(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
HOSTILE_REGISTRY_TOKEN=$(hostile_secret oras seed)
HOSTILE_HELM_TOKEN=$(hostile_secret helm seed)
HOSTILE_VERIFIER="$(hostile_secret verifier seed | cut -c1-42)A"
HOSTILE_RAW_TOKEN=$(hostile_secret github seed)
[[ "$HOSTILE_VERIFIER" =~ ^[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$ ]]
printf safe > "$2/safe"
assert_no_hostile_leaks "$2"
printf '%s' "$HOSTILE_HELM_TOKEN" > "$2/leak"
if (assert_no_hostile_leaks "$2"); then exit 1; fi
`, "hostile-credential-test", publisher, t.TempDir())
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("hostile credential leak scanner is not fail-closed: %v (%s)", err, output)
	}
}

func TestDisposableCleanupIsExactAndIdempotent(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
resources="$2/resources"
mkdir -p "$resources" "$2/run" "$2/mode" "$2/config" "$2/cache"
touch "$resources/container" "$resources/builder" "$resources/network"
docker() {
  local kind command
  if [ "$1" = buildx ]; then kind=builder; command=$2
  elif [ "$1" = network ]; then kind=network; command=$2
  else kind=container; command=$2
  fi
  if [ "$command" = inspect ]; then
    [ -e "$resources/$kind" ] && return 0
    printf 'No such object\n' >&2
    return 1
  fi
  rm -f "$resources/$kind"
}
RUN_CONTAINER=cleanup-registry
RUN_BUILDER=cleanup-builder
RUN_NETWORK=cleanup-network
RUN_TMP="$2/run"
MODE_TMP="$2/mode"
CLIENT_CONFIG_ROOT="$2/config"
OWNED_CACHE_ROOT="$2/cache"
cleanup_all
cleanup_all
[ -z "$(find "$resources" -type f -print -quit)" ]
for path in "$RUN_TMP" "$MODE_TMP" "$CLIENT_CONFIG_ROOT" "$OWNED_CACHE_ROOT"; do [ ! -e "$path" ]; done
`, "cleanup-test", publisher, tmp)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("disposable cleanup did not remove exact resources: %v (%s)", err, output)
	}
}

func TestCleanupTrapAndFailureControlFlow(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		scenario string
		exitCode int
		success  bool
	}{
		{name: "successful EXIT", scenario: "success", exitCode: 0, success: true},
		{name: "primary failure EXIT", scenario: "primary", exitCode: 7},
		{name: "INT", scenario: "int", exitCode: 130},
		{name: "TERM", scenario: "term", exitCode: 143},
		{name: "inspection failure", scenario: "inspect-failure", exitCode: 1},
		{name: "removal failure", scenario: "remove-failure", exitCode: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmp := t.TempDir()
			command := exec.Command("bash", "-c", `
set -euo pipefail
source "$1"
scenario=$2
root=$3
resources="$root/resources"
mkdir -p "$resources" "$root/run" "$root/mode" "$root/config" "$root/cache"
if [ "$scenario" != inspect-failure ]; then touch "$resources/container" "$resources/builder" "$resources/network"; fi
docker() {
  local kind command
  if [ "$1" = buildx ]; then kind=builder; command=$2
  elif [ "$1" = network ]; then kind=network; command=$2
  else kind=container; command=$2
  fi
  if [ "$command" = inspect ]; then
    if [ "$scenario" = inspect-failure ]; then printf 'daemon unavailable\n' >&2; return 1; fi
    [ -e "$resources/$kind" ] && return 0
    printf 'No such object\n' >&2
    return 1
  fi
  rm -f "$resources/$kind"
  [ "$scenario" != remove-failure ] || return 1
}
RUN_CONTAINER=cleanup-registry
RUN_BUILDER=cleanup-builder
RUN_NETWORK=cleanup-network
RUN_TMP="$root/run"
MODE_TMP="$root/mode"
CLIENT_CONFIG_ROOT="$root/config"
OWNED_CACHE_ROOT="$root/cache"
SUCCESS_MESSAGE=cleanup-success
case $scenario in
  success) trap cleanup_on_exit EXIT; exit 0 ;;
  primary) trap cleanup_on_exit EXIT; exit 7 ;;
  int) cleanup_on_signal 130 ;;
  term) cleanup_on_signal 143 ;;
  inspect-failure|remove-failure) trap cleanup_on_exit EXIT; exit 0 ;;
esac
`, "cleanup-control-test", publisher, test.scenario, tmp)
			output, runErr := command.CombinedOutput()
			actualCode := 0
			if runErr != nil {
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					actualCode = exitErr.ExitCode()
				} else {
					t.Fatal(runErr)
				}
			}
			if actualCode != test.exitCode {
				t.Fatalf("exit code = %d, want %d (%s)", actualCode, test.exitCode, output)
			}
			if strings.Contains(string(output), "cleanup-success") != test.success {
				t.Fatalf("success reporting mismatch: %q", output)
			}
			for _, path := range []string{"run", "mode", "config", "cache"} {
				if _, err := os.Lstat(filepath.Join(tmp, path)); !os.IsNotExist(err) {
					t.Fatalf("%s residue remains: %v", path, err)
				}
			}
			entries, err := os.ReadDir(filepath.Join(tmp, "resources"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("resource residue remains: %v %v (%s)", entries, err, output)
			}
		})
	}
}
