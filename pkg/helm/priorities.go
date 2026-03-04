package helm

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// getRepoPrioritiesPath returns the path to the priorities config file
func getRepoPrioritiesPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE") // Windows
	}
	return filepath.Join(home, ".config", "kubikles", "helm-repo-priorities.json")
}

// loadRepoPriorities loads repository priorities from config
func loadRepoPriorities() (*RepoPriorities, error) {
	path := getRepoPrioritiesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &RepoPriorities{Priorities: make(map[string]int)}, nil
		}
		return nil, err
	}

	var priorities RepoPriorities
	if err := json.Unmarshal(data, &priorities); err != nil {
		return nil, err
	}

	if priorities.Priorities == nil {
		priorities.Priorities = make(map[string]int)
	}

	return &priorities, nil
}

// saveRepoPriorities saves repository priorities to config
func saveRepoPriorities(priorities *RepoPriorities) error {
	path := getRepoPrioritiesPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(priorities, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// getOCIPrioritiesPath returns the path to the OCI priorities config file
func getOCIPrioritiesPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE") // Windows
	}
	return filepath.Join(home, ".config", "kubikles", "oci-registry-priorities.json")
}

// loadOCIPriorities loads OCI registry priorities from config
func loadOCIPriorities() (*OCIPriorities, error) {
	path := getOCIPrioritiesPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &OCIPriorities{Priorities: make(map[string]int)}, nil
		}
		return nil, err
	}

	var priorities OCIPriorities
	if err := json.Unmarshal(data, &priorities); err != nil {
		return nil, err
	}

	if priorities.Priorities == nil {
		priorities.Priorities = make(map[string]int)
	}

	return &priorities, nil
}

// saveOCIPriorities saves OCI registry priorities to config
func saveOCIPriorities(priorities *OCIPriorities) error {
	path := getOCIPrioritiesPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(priorities, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600) // Config may contain credentials
}
