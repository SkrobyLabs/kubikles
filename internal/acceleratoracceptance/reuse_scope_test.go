package acceleratoracceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceLifecycleScopeIsPinnedBeforeLaterFeatures(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "scripts", "check-accelerator-desktop-lifecycle-scope.sh"))
	if err != nil {
		t.Fatal("read lifecycle scope helper")
	}
	text := string(source)
	for _, exact := range []string{
		`lifecycle_head="a5217ab3baba648189bb8ff2fe927380a849e7ff"`,
		`git merge-base --is-ancestor "$lifecycle_head" HEAD`,
		`git diff --check "$base...$lifecycle_head"`,
		`git diff --name-only "$base...$lifecycle_head"`,
		`git diff --unified=0 "$base...$lifecycle_head"`,
	} {
		if !strings.Contains(text, exact) {
			t.Fatal("lifecycle scope is not pinned to its reviewed squash")
		}
	}
	if strings.Contains(text, `git diff --check "$base...HEAD"`) || strings.Contains(text, `git diff --name-only "$base...HEAD"`) {
		t.Fatal("lifecycle scope still audits later prerequisite features")
	}
}

func TestIntegratedRoutingUsesOwnedFixtureInAcceptanceReuse(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "scripts", "test-accelerator-integrated-routing-kind.sh"))
	if err != nil {
		t.Fatal("read integrated routing harness")
	}
	text := string(source)
	standalone := strings.Index(text, `if [ "$reuse_mode" = standalone ]; then`)
	sourceImage := strings.Index(text, `fail "source-image-revision-mismatch"`)
	reuse := strings.Index(text, `test "${BUILD_VERSION-}" = v0.0.0 || fail "reuse-build-version"`)
	fixture := strings.Index(text, `accelerator_e2e_validate_reused_fixture || fail "reuse-fixture"`)
	if standalone < 0 || sourceImage <= standalone || reuse <= sourceImage || fixture <= reuse {
		t.Fatal("integrated routing acceptance reuse entered standalone source-image validation")
	}
}

func TestAcceptanceBrowserFixtureAndRealGraceAreClosed(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	buildSource, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "build-accelerator-e2e-artifacts.sh"))
	if err != nil {
		t.Fatal("read acceptance artifact builder")
	}
	browserSource, err := os.ReadFile(filepath.Join(repoRoot, "pkg", "acceleratorprovision", "provision_kind_test.go"))
	if err != nil {
		t.Fatal("read acceptance Browser harness")
	}
	harnessSource, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "test-accelerator-desktop-provision-kind.sh"))
	if err != nil {
		t.Fatal("read acceptance Kind harness")
	}
	for _, exact := range []string{
		`frontend/dist/accelerator-browser/.kubikles-browser-v1.json`,
		`frontend/dist/accelerator-browser/assets/browser.js`,
		`frontend/dist/accelerator-browser/assets/browser.css`,
		`pkg/server/browserbootstrap/index.html`,
		`pkg/server/browserbootstrap/bootstrap.js`,
	} {
		if strings.Count(string(buildSource), exact) != 1 {
			t.Fatalf("acceptance Browser fixture owner missing %q", exact)
		}
	}
	browser := string(browserSource)
	fragmentClear := strings.Index(browser, `parsed.Fragment = ""`)
	firstNetwork := strings.Index(browser, `entry, err := client.Get(parsed.String())`)
	exchange := strings.Index(browser, `base+"/api/accelerator-browser-session"`)
	if fragmentClear < 0 || firstNetwork <= fragmentClear || exchange <= firstNetwork {
		t.Fatal("acceptance Browser ticket fragment is not cleared before network")
	}
	for _, exact := range []string{
		`"bootstrap/index.html"`, `"bootstrap/bootstrap.js"`, `"assets/browser.js"`, `"assets/browser.css"`,
		`elapsed < 119*time.Second || elapsed > 126*time.Second`,
		`startAcceptanceJobTerminalWatch`, `waitAcceptanceJobTerminalCleanup`,
		`ACCELERATOR_ACCEPTANCE_BROWSER_PROOF`,
	} {
		if !strings.Contains(browser, exact) {
			t.Fatalf("acceptance Browser proof missing %q", exact)
		}
	}
	if strings.Count(browser, `waitAcceptanceJobTerminalCleanup(`) != 2 {
		t.Fatal("acceptance Browser real grace has more than one additive observer")
	}
	harness := string(harnessSource)
	for _, exact := range []string{
		`name: accelerator-e2e-api-egress`,
		`app.kubernetes.io/name: kubikles-accelerator`,
		`cidr: $api_service_ip/32`,
		`ACCELERATOR_ACCEPTANCE_COMPOSED_KIND`,
		`go_test_tags=helm,accelerator_provision_kind,accelerator_e2e`,
		`go_test_environment=(GOTOOLCHAIN=go1.25.12 GOPROXY=off GOSUMDB=off)`,
		`env "${go_test_environment[@]}" HOME="$test_home"`,
	} {
		if !strings.Contains(harness, exact) {
			t.Fatalf("acceptance Browser harness boundary missing %q", exact)
		}
	}
}
