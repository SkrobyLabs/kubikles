//go:build !windows

package acceleratoracceptance

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestAcceptanceExternalNetworkTripwireCountsConnections(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "tripwire")
	addressPath := filepath.Join(directory, "address.json")
	countPath := filepath.Join(directory, "count.json")
	build := exec.Command("go", "build", "-o", binary, "./cmd/accelerator-e2e-tripwire")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("tripwire build: %s", output)
	}
	command := exec.Command(binary, addressPath, countPath)
	if err := command.Start(); err != nil {
		t.Fatal("tripwire start")
	}
	t.Cleanup(func() {
		_ = command.Process.Signal(syscall.SIGTERM)
		_ = command.Wait()
	})
	deadline := time.Now().Add(2 * time.Second)
	var address string
	for {
		data, err := os.ReadFile(addressPath)
		if err == nil && json.Unmarshal(data, &address) == nil && address != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tripwire address unavailable")
		}
		time.Sleep(10 * time.Millisecond)
	}
	connection, err := net.DialTimeout("tcp4", address, time.Second)
	if err != nil {
		t.Fatal("tripwire dial")
	}
	connection.Close()
	for {
		data, err := os.ReadFile(countPath)
		var count uint64
		if err == nil && json.Unmarshal(data, &count) == nil && count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tripwire count unavailable")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
