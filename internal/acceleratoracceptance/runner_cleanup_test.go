//go:build !windows

package acceleratoracceptance

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunnerCleanupEveryMutationAndSignal(t *testing.T) {
	directory := t.TempDir()
	pidPath := filepath.Join(directory, "child.pid")
	scriptPath := filepath.Join(directory, "child")
	script := "#!/usr/bin/env bash\nset -euo pipefail\nchild=''\ncleanup() { kill \"$child\" >/dev/null 2>&1 || true; wait \"$child\" >/dev/null 2>&1 || true; exit 143; }\ntrap cleanup TERM INT\nsleep 300 & child=$!\nprintf '%s\\n' \"$child\" >\"$1\"\nwait \"$child\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal("write direct child fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	executor := ExecCommandExecutor{Directory: directory}
	if code := executor.Run(ctx, scriptPath, []string{pidPath}, []string{"PATH=" + os.Getenv("PATH")}); code != FailureTimeout {
		t.Fatalf("timeout failure code=%s", code)
	}
	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal("child pid was not recorded")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 1 {
		t.Fatal("invalid child pid")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runner timeout retained a descendant process")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOwnedFixtureCleanupOnInterruptAndTerminate(t *testing.T) {
	helper, err := filepath.Abs(filepath.Join("..", "..", "scripts", "lib", "accelerator-e2e-kind.sh"))
	if err != nil {
		t.Fatal("helper path")
	}
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			directory := t.TempDir()
			state := filepath.Join(directory, "state")
			owned, err := os.MkdirTemp("/tmp", "kubikles-accelerator-e2e.signal-")
			if err != nil {
				t.Fatal("owned root")
			}
			ready := filepath.Join(directory, "ready")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal("state")
			}
			for name, value := range map[string]string{
				"kind-name": "kubikles-a60a-1-2-3\n", "registry-container": "kubikles-a60a-1-2-3-registry\n", "temp-root": owned + "\n",
			} {
				if err := os.WriteFile(filepath.Join(state, name), []byte(value), 0o600); err != nil {
					t.Fatal("state file")
				}
			}
			script := `set -euo pipefail
source "$1"
state="$2"
ready="$3"
cleanup() { trap - EXIT INT TERM; accelerator_e2e_cleanup_owned_fixture "$state"; exit 143; }
trap cleanup INT TERM
: >"$ready"
while :; do read -r -t 1 _ || true; done`
			command := exec.Command("bash", "-c", script, "_", helper, state, ready)
			if err := command.Start(); err != nil {
				t.Fatal("start signal fixture")
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				if _, err := os.Stat(ready); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("signal fixture not ready")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := command.Process.Signal(signal); err != nil {
				t.Fatal("send signal")
			}
			if err := command.Wait(); err == nil {
				t.Fatal("signal cleanup returned success")
			}
			if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("signal cleanup retained state")
			}
			if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("signal cleanup retained owned root")
			}
		})
	}
}
