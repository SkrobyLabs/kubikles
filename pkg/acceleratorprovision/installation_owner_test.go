package acceleratorprovision

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPersistentInstallationOwnerCreatesAndReloadsStableID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubikles", installationOwnerFileName)
	first := newPersistentInstallationOwner(path)
	first.entropy = bytes.NewReader([]byte("0123456789abcdef"))
	id, err := first.InstallationID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "30313233343536373839616263646566" {
		t.Fatalf("id=%q", id)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v", info.Mode().Perm())
	}

	second := newPersistentInstallationOwner(path)
	second.entropy = bytes.NewReader([]byte("different-value!"))
	reloaded, err := second.InstallationID()
	if err != nil || reloaded != id {
		t.Fatalf("reloaded=%q err=%v", reloaded, err)
	}
}

func TestPersistentInstallationOwnerConcurrentCreatorsConverge(t *testing.T) {
	path := filepath.Join(t.TempDir(), installationOwnerFileName)
	one, two := newPersistentInstallationOwner(path), newPersistentInstallationOwner(path)
	one.entropy = bytes.NewReader([]byte("0123456789abcdef"))
	two.entropy = bytes.NewReader([]byte("fedcba9876543210"))
	results := make(chan string, 2)
	errors := make(chan error, 2)
	var group sync.WaitGroup
	for _, owner := range []*persistentInstallationOwner{one, two} {
		group.Add(1)
		go func(candidate *persistentInstallationOwner) {
			defer group.Done()
			id, err := candidate.InstallationID()
			results <- id
			errors <- err
		}(owner)
	}
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var want string
	for id := range results {
		if want == "" {
			want = id
		}
		if id != want {
			t.Fatalf("concurrent IDs diverged: %q != %q", id, want)
		}
	}
}

func TestPersistentInstallationOwnerRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), installationOwnerFileName)
	if err := os.WriteFile(path, []byte("not-an-owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	owner := newPersistentInstallationOwner(path)
	if _, err := owner.InstallationID(); err == nil {
		t.Fatal("corrupt installation owner was replaced")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "not-an-owner\n" {
		t.Fatalf("corrupt owner changed: %q err=%v", data, err)
	}
}
