package ai

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "kubikles"
	keyringKey     = "anthropic-api-key"
	keyFileName    = "anthropic_key" // fallback when keyring unavailable
)

// resolveAnthropicAPIKey returns the Anthropic API key.
// Priority: env var → OS keyring → file fallback.
func resolveAnthropicAPIKey() string {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key
	}
	if key, err := keyring.Get(keyringService, keyringKey); err == nil && key != "" {
		return key
	}
	return readFallbackFile()
}

// SaveAnthropicAPIKey stores the API key in the OS keyring.
// Falls back to a plain-text file if keyring is unavailable.
func SaveAnthropicAPIKey(key string) error {
	trimmed := strings.TrimSpace(key)
	if err := keyring.Set(keyringService, keyringKey, trimmed); err != nil {
		log.Printf("[AI] OS keyring unavailable, falling back to file: %v", err)
		return saveFallbackFile(trimmed)
	}
	return nil
}

// ClearAnthropicAPIKey removes the API key from keyring and fallback file.
func ClearAnthropicAPIKey() error {
	if err := keyring.Delete(keyringService, keyringKey); err != nil && err != keyring.ErrNotFound {
		log.Printf("[AI] Failed to delete from keyring: %v", err)
	}
	removeFallbackFile()
	return nil
}

// GetAnthropicAPIKeyStatus returns "env", "configured", or "not_set".
func GetAnthropicAPIKeyStatus() string {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "env"
	}
	if key, err := keyring.Get(keyringService, keyringKey); err == nil && key != "" {
		return "configured"
	}
	if readFallbackFile() != "" {
		return "configured"
	}
	return "not_set"
}

// --- file fallback (used when OS keyring is unavailable) ---

func fallbackFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "kubikles", keyFileName), nil
}

func readFallbackFile() string {
	path, err := fallbackFilePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveFallbackFile(key string) error {
	path, err := fallbackFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(key), 0600)
}

func removeFallbackFile() {
	if path, err := fallbackFilePath(); err == nil {
		os.Remove(path)
	}
}
