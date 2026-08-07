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
