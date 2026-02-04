package main

import (
	"fmt"
)

// =============================================================================
// Themes
// =============================================================================

// GetThemes returns all available themes
func (a *App) GetThemes() []Theme {
	if a.themeManager == nil {
		return []Theme{}
	}
	return a.themeManager.GetThemes()
}

// GetCurrentTheme returns the currently active theme
func (a *App) GetCurrentTheme() *Theme {
	if a.themeManager == nil {
		return nil
	}
	return a.themeManager.GetCurrentTheme()
}

// SetTheme switches to the specified theme
func (a *App) SetTheme(themeID string) error {
	if a.themeManager == nil {
		return fmt.Errorf("theme manager not initialized")
	}
	return a.themeManager.SetTheme(themeID)
}

// ReloadThemes reloads user themes from disk
func (a *App) ReloadThemes() []Theme {
	if a.themeManager == nil {
		return []Theme{}
	}
	return a.themeManager.ReloadUserThemes()
}

// GetThemesDir returns the user themes directory path
func (a *App) GetThemesDir() string {
	if a.themeManager == nil {
		return ""
	}
	return a.themeManager.GetThemesDir()
}

// OpenThemesDir opens the themes directory in the file manager
func (a *App) OpenThemesDir() error {
	if a.themeManager == nil {
		return fmt.Errorf("theme manager not initialized")
	}
	return a.themeManager.OpenThemesDir()
}
