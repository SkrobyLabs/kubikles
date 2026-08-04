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
