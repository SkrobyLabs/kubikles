package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
)

// ThemeColors represents theme color definitions
type ThemeColors struct {
	Background          string `json:"background"`
	BackgroundDark      string `json:"backgroundDark"`
	Surface             string `json:"surface"`
	SurfaceLight        string `json:"surfaceLight"`
	SurfaceHover        string `json:"surfaceHover"`
	Primary             string `json:"primary"`
	Text                string `json:"text"`
	TextMuted           string `json:"textMuted"`
	Border              string `json:"border"`
	Success             string `json:"success"`
	SuccessDark         string `json:"successDark"`
	Error               string `json:"error"`
	ErrorDark           string `json:"errorDark"`
	Warning             string `json:"warning"`
	WarningDark         string `json:"warningDark"`
	RedOrange           string `json:"redOrange"`
	RedOrangeDark       string `json:"redOrangeDark"`
	ScrollbarTrack      string `json:"scrollbarTrack"`
	ScrollbarThumb      string `json:"scrollbarThumb"`
	ScrollbarThumbHover string `json:"scrollbarThumbHover"`
}

// ThemeFontConfig represents font configuration
type ThemeFontConfig struct {
	Family  string `json:"family"`
	Weights []int  `json:"weights,omitempty"`
}

// ThemeFonts represents theme font definitions
type ThemeFonts struct {
	UI   ThemeFontConfig `json:"ui"`
	Mono ThemeFontConfig `json:"mono"`
}

// Theme represents a complete theme definition
type Theme struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Version     string      `json:"version,omitempty"`
	Author      string      `json:"author,omitempty"`
	Description string      `json:"description,omitempty"`
	Colors      ThemeColors `json:"colors"`
	Fonts       ThemeFonts  `json:"fonts"`
	IsBuiltin   bool        `json:"isBuiltin"`
	FilePath    string      `json:"filePath,omitempty"`
}

// ThemeManager manages theme loading and switching
type ThemeManager struct {
	app            *App
	themes         map[string]*Theme
	currentThemeID string
	configPath     string
	themesDir      string
	mu             sync.RWMutex
}

// ThemeConfig persisted to disk
type ThemeConfig struct {
	SelectedTheme string `json:"selectedTheme"`
}

// NewThemeManager creates a new theme manager
func NewThemeManager(app *App, configDir string) *ThemeManager {
	m := &ThemeManager{
		app:        app,
		themes:     make(map[string]*Theme),
		configPath: filepath.Join(configDir, "theme_config.json"),
		themesDir:  filepath.Join(configDir, "themes"),
	}

	// Ensure themes directory exists
	os.MkdirAll(m.themesDir, 0755)

	m.loadBuiltinThemes()
	m.loadUserThemes()
	m.loadConfig()

	return m
}

// loadBuiltinThemes loads the built-in themes
func (m *ThemeManager) loadBuiltinThemes() {
	// Default Dark theme (current VS Code-inspired theme)
	m.themes["default-dark"] = &Theme{
		ID:          "default-dark",
		Name:        "Default Dark",
		Version:     "1.0.0",
		Author:      "Kubikles",
		Description: "The default dark theme inspired by VS Code",
		IsBuiltin:   true,
		Colors: ThemeColors{
			Background:          "#1e1e1e",
			BackgroundDark:      "#1a1a1a",
			Surface:             "#252526",
			SurfaceLight:        "#2d2d2d",
			SurfaceHover:        "#3d3d3d",
			Primary:             "#007acc",
			Text:                "#cccccc",
			TextMuted:           "#808080",
			Border:              "#3e3e42",
			Success:             "#4CC38A",
			SuccessDark:         "#3AA876",
			Error:               "#E5484D",
			ErrorDark:           "#C33A3F",
			Warning:             "#F5A623",
			WarningDark:         "#D98C1C",
			RedOrange:           "#E66B2F",
			RedOrangeDark:       "#C75A27",
			ScrollbarTrack:      "#252526",
			ScrollbarThumb:      "#3e3e42",
			ScrollbarThumbHover: "#007acc",
		},
		Fonts: ThemeFonts{
			UI:   ThemeFontConfig{Family: "'Inter', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"},
			Mono: ThemeFontConfig{Family: "'JetBrains Mono', 'Menlo', 'Monaco', 'Consolas', monospace"},
		},
	}

	// Solarized Dark theme - Ethan Schoonover's palette
	m.themes["solarized-dark"] = &Theme{
		ID:          "solarized-dark",
		Name:        "Solarized Dark",
		Version:     "1.0.0",
		Author:      "Kubikles",
		Description: "Warm, earthy tones with reduced eye strain based on Solarized by Ethan Schoonover",
		IsBuiltin:   true,
		Colors: ThemeColors{
			Background:          "#002b36", // base03
			BackgroundDark:      "#00212b", // darker than base03
			Surface:             "#073642", // base02
			SurfaceLight:        "#094552", // slightly lighter than base02
			SurfaceHover:        "#0a5264", // hover state
			Primary:             "#268bd2", // blue
			Text:                "#839496", // base0
			TextMuted:           "#586e75", // base01
			Border:              "#073642", // base02
			Success:             "#859900", // green
			SuccessDark:         "#6b7d00",
			Error:               "#dc322f", // red
			ErrorDark:           "#b32825",
			Warning:             "#b58900", // yellow
			WarningDark:         "#906d00",
			RedOrange:           "#cb4b16", // orange
			RedOrangeDark:       "#a33d12",
			ScrollbarTrack:      "#073642",
			ScrollbarThumb:      "#586e75",
			ScrollbarThumbHover: "#268bd2",
		},
		Fonts: ThemeFonts{
			UI:   ThemeFontConfig{Family: "'Inter', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"},
			Mono: ThemeFontConfig{Family: "'JetBrains Mono', 'Menlo', 'Monaco', 'Consolas', monospace"},
		},
	}

	// Solarized Midnight - deeper, darker variant with brighter text
	m.themes["solarized-midnight"] = &Theme{
		ID:          "solarized-midnight",
		Name:        "Solarized Midnight",
		Version:     "1.0.0",
		Author:      "Kubikles",
		Description: "A deeper, darker Solarized variant with enhanced contrast and brighter text",
		IsBuiltin:   true,
		Colors: ThemeColors{
			Background:          "#001820", // very deep blue-black
			BackgroundDark:      "#00121a", // even darker
			Surface:             "#002030", // dark surface
			SurfaceLight:        "#002b40", // slightly lighter
			SurfaceHover:        "#003550", // hover state
			Primary:             "#2aa198", // cyan (warmer than blue)
			Text:                "#93a1a1", // base1 (brighter than base0)
			TextMuted:           "#657b83", // base00 (brighter than base01)
			Border:              "#003040", // subtle border
			Success:             "#859900", // green
			SuccessDark:         "#6b7d00",
			Error:               "#dc322f", // red
			ErrorDark:           "#b32825",
			Warning:             "#b58900", // yellow
			WarningDark:         "#906d00",
			RedOrange:           "#cb4b16", // orange
			RedOrangeDark:       "#a33d12",
			ScrollbarTrack:      "#002030",
			ScrollbarThumb:      "#003550",
			ScrollbarThumbHover: "#2aa198",
		},
		Fonts: ThemeFonts{
			UI:   ThemeFontConfig{Family: "'Inter', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"},
			Mono: ThemeFontConfig{Family: "'JetBrains Mono', 'Menlo', 'Monaco', 'Consolas', monospace"},
		},
	}

	// Phosphor - gentle CRT terminal aesthetic with soft amber and green
	m.themes["phosphor"] = &Theme{
		ID:          "phosphor",
		Name:        "Phosphor",
		Version:     "1.0.0",
		Author:      "Kubikles",
		Description: "Gentle hacker aesthetic inspired by vintage CRT terminals with soft phosphor glow",
		IsBuiltin:   true,
		Colors: ThemeColors{
			Background:          "#0a0f0a", // very dark with slight green tint
			BackgroundDark:      "#080c08", // even darker
			Surface:             "#0f1610", // dark surface with green undertone
			SurfaceLight:        "#151f17", // slightly lighter
			SurfaceHover:        "#1a2a1c", // hover state
			Primary:             "#d4a056", // soft amber/gold
			Text:                "#7ec87e", // soft phosphor green (not neon)
			TextMuted:           "#4a7a4a", // muted green
			Border:              "#1a2a1c", // subtle green border
			Success:             "#5cb85c", // brighter green for success
			SuccessDark:         "#4a9a4a",
			Error:               "#c9634a", // muted coral/rust (not harsh red)
			ErrorDark:           "#a8523d",
			Warning:             "#d4a056", // amber
			WarningDark:         "#b8883a",
			RedOrange:           "#c97849", // soft orange
			RedOrangeDark:       "#a8623a",
			ScrollbarTrack:      "#0f1610",
			ScrollbarThumb:      "#1a2a1c",
			ScrollbarThumbHover: "#d4a056",
		},
		Fonts: ThemeFonts{
			UI:   ThemeFontConfig{Family: "'Inter', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"},
			Mono: ThemeFontConfig{Family: "'JetBrains Mono', 'Menlo', 'Monaco', 'Consolas', monospace"},
		},
	}
}

// loadUserThemes loads themes from ~/.config/kubikles/themes/
func (m *ThemeManager) loadUserThemes() {
	entries, err := os.ReadDir(m.themesDir)
	if err != nil {
		// Directory doesn't exist or can't be read, that's fine
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}

		themePath := filepath.Join(m.themesDir, entry.Name())
		data, err := os.ReadFile(themePath)
		if err != nil {
			if m.app != nil {
				m.app.LogDebug("Theme: Failed to read theme file %s: %v", themePath, err)
			}
			continue
		}

		var theme Theme
		if err := json.Unmarshal(data, &theme); err != nil {
			if m.app != nil {
				m.app.LogDebug("Theme: Failed to parse theme file %s: %v", themePath, err)
			}
			continue
		}

		// Validate required fields
		if theme.ID == "" || theme.Name == "" {
			if m.app != nil {
				m.app.LogDebug("Theme: Invalid theme file %s: missing id or name", themePath)
			}
			continue
		}

		theme.IsBuiltin = false
		theme.FilePath = themePath
		m.themes[theme.ID] = &theme

		if m.app != nil {
			m.app.LogDebug("Theme: Loaded user theme '%s' from %s", theme.Name, themePath)
		}
	}
}

// loadConfig loads the persisted theme selection
func (m *ThemeManager) loadConfig() {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		m.currentThemeID = "default-dark"
		return
	}

	var config ThemeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		m.currentThemeID = "default-dark"
		return
	}

	if _, exists := m.themes[config.SelectedTheme]; exists {
		m.currentThemeID = config.SelectedTheme
	} else {
		m.currentThemeID = "default-dark"
	}
}

// saveConfig persists the theme selection
func (m *ThemeManager) saveConfig() error {
	config := ThemeConfig{SelectedTheme: m.currentThemeID}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath, data, 0644)
}

// GetThemes returns all available themes (called from frontend)
func (m *ThemeManager) GetThemes() []Theme {
	m.mu.RLock()
	defer m.mu.RUnlock()

	themes := make([]Theme, 0, len(m.themes))
	for _, t := range m.themes {
		themes = append(themes, *t)
	}

	// Sort: builtins first (alphabetically), then user themes (alphabetically)
	sort.Slice(themes, func(i, j int) bool {
		if themes[i].IsBuiltin != themes[j].IsBuiltin {
			return themes[i].IsBuiltin
		}
		return themes[i].Name < themes[j].Name
	})

	return themes
}

// GetCurrentTheme returns the current theme (called from frontend)
func (m *ThemeManager) GetCurrentTheme() *Theme {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if theme, exists := m.themes[m.currentThemeID]; exists {
		return theme
	}
	return m.themes["default-dark"]
}

// GetCurrentThemeID returns the current theme ID
func (m *ThemeManager) GetCurrentThemeID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentThemeID
}

// SetTheme switches to a theme and emits an event (called from menu or frontend)
func (m *ThemeManager) SetTheme(themeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	theme, exists := m.themes[themeID]
	if !exists {
		return fmt.Errorf("theme '%s' not found", themeID)
	}

	m.currentThemeID = themeID
	if err := m.saveConfig(); err != nil {
		if m.app != nil {
			m.app.LogDebug("Theme: Failed to save config: %v", err)
		}
	}

	// Emit event to frontend
	if m.app != nil {
		m.app.emitEvent("theme:changed", theme)
	}

	return nil
}

// ReloadUserThemes reloads themes from disk (for hot-reload)
func (m *ThemeManager) ReloadUserThemes() []Theme {
	m.mu.Lock()
	// Clear non-builtin themes
	for id, theme := range m.themes {
		if !theme.IsBuiltin {
			delete(m.themes, id)
		}
	}
	m.mu.Unlock()

	m.loadUserThemes()

	themes := m.GetThemes()

	// Emit event to frontend
	if m.app != nil {
		m.app.emitEvent("theme:list-changed", themes)
	}

	return themes
}

// GetThemesDir returns the user themes directory path
func (m *ThemeManager) GetThemesDir() string {
	return m.themesDir
}

// OpenThemesDir opens the themes directory in the file manager
func (m *ThemeManager) OpenThemesDir() error {
	// Windows needs file:/// (three slashes) for local paths
	// Unix uses file:// (two slashes)
	var url string
	if goruntime.GOOS == "windows" {
		// Convert backslashes to forward slashes and use file:/// prefix
		url = "file:///" + strings.ReplaceAll(m.themesDir, "\\", "/")
	} else {
		url = "file://" + m.themesDir
	}
	m.app.openBrowserURL(url)
	return nil
}
