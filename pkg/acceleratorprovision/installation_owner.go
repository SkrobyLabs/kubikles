package acceleratorprovision

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"kubikles/pkg/debug"
)

const installationOwnerFileName = "accelerator-installation-id"

var installationOwnerID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type installationOwnerProvider interface {
	InstallationID() (string, error)
}

type staticInstallationOwner string

func (o staticInstallationOwner) InstallationID() (string, error) {
	id := string(o)
	if !installationOwnerID.MatchString(id) {
		return "", fmt.Errorf("invalid Accelerator installation owner ID")
	}
	return id, nil
}

type persistentInstallationOwner struct {
	mu      sync.Mutex
	path    string
	entropy io.Reader
	id      string
}

func newPersistentInstallationOwner(path string) *persistentInstallationOwner {
	return &persistentInstallationOwner{path: path, entropy: rand.Reader}
}

func (o *persistentInstallationOwner) InstallationID() (string, error) {
	if o == nil {
		return "", fmt.Errorf("Accelerator installation ownership is unavailable")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.id != "" {
		return o.id, nil
	}
	path, err := o.ownerPath()
	if err != nil {
		return "", err
	}
	if id, found, readErr := readInstallationOwner(path); readErr != nil {
		debug.LogHelm("Accelerator installation ownership failed", map[string]interface{}{"path": path, "error": readErr.Error()})
		return "", readErr
	} else if found {
		o.id = id
		debug.LogHelm("Accelerator installation ownership loaded", map[string]interface{}{"installationId": id, "path": path})
		return id, nil
	}
	id, err := generateInstallationOwner(o.entropy)
	if err != nil {
		return "", err
	}
	if err := persistInstallationOwner(path, id); err != nil {
		debug.LogHelm("Accelerator installation ownership failed", map[string]interface{}{"path": path, "error": err.Error()})
		return "", err
	}
	// Another Kubikles process may have won the atomic create. Always read the
	// installed value instead of assuming this process's candidate won.
	installed, found, err := readInstallationOwner(path)
	if err != nil || !found {
		if err == nil {
			err = fmt.Errorf("Accelerator installation owner ID was not persisted")
		}
		return "", err
	}
	o.id = installed
	debug.LogHelm("Accelerator installation ownership created", map[string]interface{}{"installationId": installed, "path": path})
	return installed, nil
}

func (o *persistentInstallationOwner) ownerPath() (string, error) {
	if o.path != "" {
		return o.path, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve Accelerator installation owner path: %w", err)
	}
	return filepath.Join(configDir, "kubikles", installationOwnerFileName), nil
}

func readInstallationOwner(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read Accelerator installation owner ID: %w", err)
	}
	id := strings.TrimSpace(string(data))
	if !installationOwnerID.MatchString(id) {
		return "", false, fmt.Errorf("Accelerator installation owner ID is invalid")
	}
	return id, true, nil
}

func generateInstallationOwner(entropy io.Reader) (string, error) {
	if entropy == nil {
		return "", fmt.Errorf("Accelerator installation ownership entropy is unavailable")
	}
	value := make([]byte, 16)
	if _, err := io.ReadFull(entropy, value); err != nil {
		return "", fmt.Errorf("generate Accelerator installation owner ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func persistInstallationOwner(path, id string) error {
	if path == "" || !installationOwnerID.MatchString(id) {
		return fmt.Errorf("invalid Accelerator installation owner persistence request")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Accelerator installation owner directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".accelerator-installation-id-*")
	if err != nil {
		return fmt.Errorf("create Accelerator installation owner temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure Accelerator installation owner temporary file: %w", err)
	}
	if _, err := io.WriteString(temporary, id+"\n"); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write Accelerator installation owner ID: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync Accelerator installation owner ID: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Accelerator installation owner ID: %w", err)
	}
	// A hard link publishes a fully written file without replacing an ID that
	// another Kubikles process may have installed concurrently.
	if err := os.Link(temporaryPath, path); err != nil && !os.IsExist(err) {
		return fmt.Errorf("publish Accelerator installation owner ID: %w", err)
	}
	return nil
}
