package helm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Pre-compiled regex for ACR URL parsing (avoid recompiling on every call)
var acrURLRegex = regexp.MustCompile(`https?://([^.]+)\.azurecr\.io`)

// isACRURL checks if a URL is an Azure Container Registry URL
func isACRURL(url string) bool {
	return strings.Contains(url, ".azurecr.io")
}

// extractACRName extracts the registry name from an ACR URL (with protocol prefix)
func extractACRName(url string) string {
	matches := acrURLRegex.FindStringSubmatch(url)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractACRNameFromURL extracts the registry name from an ACR URL (with or without protocol)
func extractACRNameFromURL(url string) string {
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	if strings.Contains(url, ".azurecr.io") {
		parts := strings.Split(url, ".")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

// isJWTExpired checks if a JWT token is expired (with 5 minute buffer)
func isJWTExpired(token string) bool {
	if token == "" {
		return true
	}

	// JWT has 3 parts separated by dots
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		// Not a valid JWT - might be a static password, treat as not expired
		return false
	}

	// Decode the payload (second part) - JWT uses base64url encoding
	// Add padding if needed
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try standard encoding as fallback
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return false // Can't decode, assume not expired
		}
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return false // Can't parse, assume not expired
	}

	now := time.Now().Unix()
	return now > (claims.Exp - 300)
}

// refreshACRCredentials attempts to get ACR admin credentials using Azure CLI
func refreshACRCredentials(registryName string) (*ACRCredentials, error) {
	// Try to get admin credentials (more reliable than refresh tokens)
	cmd := exec.Command("az", "acr", "credential", "show", "-n", registryName, //nolint:gosec
		"--query", "{username:username, password:passwords[0].value}", "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		// Admin might be disabled, try token approach as fallback
		return refreshACRToken(registryName)
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(output, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse ACR credentials: %w", err)
	}

	if creds.Username == "" || creds.Password == "" {
		return refreshACRToken(registryName)
	}

	return &ACRCredentials{Username: creds.Username, Password: creds.Password}, nil
}

// refreshACRToken attempts to get a fresh ACR token using Azure CLI (fallback)
func refreshACRToken(registryName string) (*ACRCredentials, error) {
	// Try to get token using az acr login --expose-token
	cmd := exec.Command("az", "acr", "login", "-n", registryName, "--expose-token", "--query", "accessToken", "-o", "tsv") //nolint:gosec
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ACR credentials (is Azure CLI installed and logged in?): %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return nil, fmt.Errorf("empty token returned from Azure CLI")
	}

	// Token auth uses special username
	return &ACRCredentials{
		Username: "00000000-0000-0000-0000-000000000000",
		Password: token,
	}, nil
}
