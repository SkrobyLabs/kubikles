package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func workflow(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var parsed any
	if err := yaml.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("%s is invalid YAML: %v", name, err)
	}
	return string(b)
}

func TestWorkflowReleaseContract(t *testing.T) {
	build := workflow(t, "build.yml")
	pr := workflow(t, "pull-request.yml")
	release := workflow(t, "release.yml")
	installer, err := os.ReadFile(filepath.Join("..", "install-accelerator-release-tools.sh"))
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := os.ReadFile(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	all := build + "\n" + pr + "\n" + release + "\n" + string(installer) + "\n" + string(publisher)

	for _, required := range []string{
		"build_version:", "default: dev", "BUILD_VERSION='${{ inputs.build_version }}'",
		"permissions:\n  contents: read", "publish: false", "build_version: dev",
		"group: release-${{ github.ref_name }}", "cancel-in-progress: false", "^v(0|[1-9][0-9]*)",
		"packages: write", "contents: write", "actions: read", "packages: read",
		"Docker Buildx must be exactly v0.36.0", "helm-v3.21.3", "oras_1.3.3", "gh_2.97.0",
		"kubikles-accelerator-release-$tag.json", "make verify-accelerator-release-ghcr",
		"verify-release-assets", "GitHub Release could not be classified authoritatively",
		"Install checksum-pinned Helm for release contracts", "helm-v3.21.3-linux-amd64.tar.gz",
		"15e041a93a590dce8100f39385cd98c84a765c9e36aeeb9e2dc6ff9e4769e2e0", `>> "$GITHUB_PATH"`,
	} {
		if !strings.Contains(all, required) {
			t.Errorf("workflow contract missing %q", required)
		}
	}
	for _, forbidden := range []string{"gh release delete", "release edit", "--clobber", "cancel-in-progress: true", "packages: write\n\njobs:\n  build", ":latest", "stable channel"} {
		if strings.Contains(strings.ToLower(all), strings.ToLower(forbidden)) {
			t.Errorf("workflow contains forbidden policy %q", forbidden)
		}
	}
	installHelm := strings.Index(build, "Install checksum-pinned Helm for release contracts")
	contractTests := strings.Index(build, "make test-accelerator-release-contract")
	if installHelm < 0 || contractTests < 0 || installHelm > contractTests {
		t.Fatal("read-only build workflow does not install pinned Helm before release contracts")
	}
	action := regexp.MustCompile(`uses:\s+[^\s#]+@([^\s#]+)`)
	for _, match := range action.FindAllStringSubmatch(all, -1) {
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(match[1]) {
			t.Errorf("action is not commit-pinned: %s", match[0])
		}
	}
	for _, pin := range []string{
		"actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683",
		"actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5",
		"actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020",
		"actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
		"actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093",
		"docker/setup-buildx-action@e468171a9de216ec08956ac3ada2f0791b6bd435",
		"docker/login-action@184bdaa0721073962dff0199f1fb9940f07167d1",
	} {
		if !strings.Contains(all, pin) {
			t.Errorf("missing audited action pin %s", pin)
		}
	}
}

func TestDesktopReleaseAssetsRemainExact(t *testing.T) {
	build := workflow(t, "build.yml")
	release := workflow(t, "release.yml")
	assets := []string{"Kubikles-windows-amd64.zip", "Kubikles-windows-arm64.zip", "Kubikles-macos-arm64.zip", "Kubikles-macos-amd64.zip", "Kubikles-linux-amd64.zip"}
	for _, asset := range assets {
		if !strings.Contains(build, asset) || !strings.Contains(release, asset) {
			t.Errorf("desktop asset %s was not preserved", asset)
		}
	}
	exact := append(append([]string(nil), assets...), "kubikles-accelerator-release-v1.4.2.json", "kubikles-accelerator-release-v1.4.2.json.sha256")
	if err := VerifyReleaseAssetNames("v1.4.2", exact); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(release, `find "$RUNNER_TEMP/artifacts" -type f -printf '%f\n'`) || strings.Contains(release, `--pattern "kubikles-accelerator-release-$tag.json*"`) {
		t.Fatal("workflow does not enforce the exact finalized asset set")
	}
}

func TestOwnedPublicationScope(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, forbidden := range []string{"kubikles-agent", "Dockerfile.agent", "latest", "--force", "--overwrite", "--delete", "cosign", "sbom=true", "provenance=true"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Errorf("publication script contains out-of-scope marker %q", forbidden)
		}
	}
}

func TestPublisherUsesHardenedReleaseInspectionArguments(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	inspectCalls := regexp.MustCompile(`(?m)^\s*go run \./scripts/accelerator-release inspect-chart ([^\n]+)$`).FindAllStringSubmatch(text, -1)
	if len(inspectCalls) != 4 {
		t.Fatalf("inspect-chart call count = %d, want 4", len(inspectCalls))
	}
	for _, call := range inspectCalls {
		if !strings.Contains(call[1], `"$CHART_SOURCE"`) || (!strings.Contains(call[1], `"$epoch"`) && !strings.Contains(call[1], `"$SOURCE_DATE_EPOCH"`)) {
			t.Errorf("inspect-chart call omits exact chart source or release epoch: %s", call[0])
		}
	}
	if !strings.Contains(text, `"$tmp/first" "$epoch" absent`) || !strings.Contains(text, `"$tmp/second" "$epoch" absent`) || !strings.Contains(text, `"$work/publication" "$SOURCE_DATE_EPOCH" "$release_state"`) {
		t.Fatal("publish_registry does not propagate release identity and finalization state")
	}
	if strings.Count(text, `go run ./scripts/accelerator-release verify-git`) != 3 {
		t.Fatal("publication and read-back do not use hardened Git source authority")
	}
	release := workflow(t, "release.yml")
	if !strings.Contains(release, `commit="$(go run ./scripts/accelerator-release verify-git . "$tag")"`) {
		t.Fatal("release preflight does not use hardened Git source authority")
	}
	if strings.Contains(release, "\n          path: artifacts\n") || strings.Contains(release, "> notes.md") {
		t.Fatal("release workflow dirties the verified source checkout with generated artifacts")
	}
}

func TestPublisherFailClosedStateMachineContract(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, forbidden := range []string{
		`resolve_or_absent`, `oras resolve "$ref" 2>/dev/null`, `resolve --plain-http "$ref" 2>/dev/null`,
		`gh release view "$tag" >/dev/null 2>&1`, `helm pull "oci://$chart_repo" --version`,
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("publisher retains ambiguous or floating classification path %q", forbidden)
		}
	}
	for _, required := range []string{
		"registry reference could not be classified authoritatively",
		"Classify and deeply compare the entire immutable set before the first write",
		"verify-registry-evidence", "inspect-chart-manifest", "helm_pull_digest",
		"remote_tag_commit", "verify-release-assets", "recheck_release_state",
		"ACCELERATOR_LOCAL_REGISTRY_CLIENT_HOST", "configured registry client endpoint is unreachable",
		"--registry-config \"$ORAS_CONFIG\"", "--registry-config \"$HELM_REGISTRY_CONFIG\"",
		"image-collision", "chart-collision", "finalized-$collision", "unreachable registry was classified as absent",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("publisher hardening contract missing %q", required)
		}
	}
	if strings.Index(text, `image_state=$(classify_registry_ref`) > strings.Index(text, `oras_copy_from_layout "$layout:$version"`) || strings.Index(text, `chart_state=$(classify_registry_ref`) > strings.Index(text, `oras_copy_from_layout "$layout:$version"`) {
		t.Fatal("registry mutation appears before full image/chart classification")
	}
}

func TestToolChecksAreCanonicalExactBuilds(t *testing.T) {
	publisher, err := os.ReadFile(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	installer, err := os.ReadFile(filepath.Join("..", "install-accelerator-release-tools.sh"))
	if err != nil {
		t.Fatal(err)
	}
	all := string(publisher) + string(installer)
	for _, exact := range []string{"v3.21.3+g1ad6e68", "210747c29c1d38732b3194878dfd8b5a6b9ad7eb", "gh version 2.97.0 (2026-07-31)"} {
		if !strings.Contains(all, exact) {
			t.Errorf("missing canonical tool build identity %q", exact)
		}
	}
	for _, loose := range []string{`^v3\.21\.3([+-]|$)`, `^gh version 2\.97\.0 `} {
		if strings.Contains(all, loose) {
			t.Errorf("loose tool version acceptance remains: %q", loose)
		}
	}
}

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
	release := workflow(t, "release.yml")
	if !strings.Contains(release, `github_token="$GH_TOKEN"`) || !strings.Contains(release, "unset GH_TOKEN") || !strings.Contains(release, `github_cli() { GH_TOKEN="$github_token" gh "$@"; }`) {
		t.Fatal("release finalizer does not narrow GitHub token inheritance")
	}
}

func TestVerifierUsesBareChartRepositoryForBlobAndHelmPull(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(publisher)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, required := range []string{
		`chart_repo=${chart_ref#oci://}; chart_repo=${chart_repo%@*}`,
		`fetch_registry_blob "$chart_repo" "$config_digest"`,
		`helm_pull_digest "$chart_repo" "$chart_digest"`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("digest-qualified chart normalization missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`fetch_registry_blob "${chart_ref#oci://}"`,
		`helm_pull_digest "${chart_ref#oci://}"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("digest-qualified chart repository is reused by %q", forbidden)
		}
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
	release := workflow(t, "release.yml")
	for _, required := range []string{".tag_name, .name, .draft, .prerelease", `release_name" = "$tag`, `release_draft" = false`, `release_prerelease" = false`} {
		if !strings.Contains(release, required) {
			t.Errorf("release finalizer metadata contract missing %q", required)
		}
	}
}

func TestHostileCredentialsUsePrivateInputsAndLeakScanner(t *testing.T) {
	publisher, err := filepath.Abs(filepath.Join("..", "publish-accelerator-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(publisher)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, required := range []string{"private/oras.json", "private/helm.json", "private/gh/hosts.yml", "render-values.yaml", "github-failure.log", "helm-render.log", "assert_no_hostile_leaks"} {
		if !strings.Contains(text, required) {
			t.Errorf("hostile credential exercise missing %q", required)
		}
	}
	for _, forbidden := range []string{"hostile-registry-token-NOT-FOR-LOGS", "hostile-verifier-NOT-FOR-LOGS", "hostile-raw-token-NOT-FOR-LOGS"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("hostile credential is a fixed source literal %q", forbidden)
		}
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
