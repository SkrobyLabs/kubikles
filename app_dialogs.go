package main

import (
	"archive/zip"
	"fmt"
	"os"

	"kubikles/pkg/debug"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// =============================================================================
// Dialogs & File Operations
// =============================================================================

// ConfirmDialog shows a confirmation dialog and returns true if the user confirms
func (a *App) ConfirmDialog(title, message string) bool {
	result, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         title,
		Message:       message,
		Buttons:       []string{"Delete", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil {
		debug.LogUI("ConfirmDialog error", map[string]interface{}{"error": err.Error()})
		return false
	}
	return result == "Delete"
}

func (a *App) SaveLogFile(content string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "kubikles-debug-logs.txt",
		Title:           "Save Debug Logs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User canceled
	}

	return os.WriteFile(filePath, []byte(content), 0644) //nolint:gosec // User-exported file, 0644 is intentional
}

func (a *App) SavePodLogs(content string, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save Pod Logs",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Log Files (*.log)",
				Pattern:     "*.log",
			},
			{
				DisplayName: "Text Files (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User canceled
	}

	return os.WriteFile(filePath, []byte(content), 0644) //nolint:gosec // User-exported file, 0644 is intentional
}

// PodLogEntry represents a single container's logs for the bundle
type PodLogEntry struct {
	PodName       string `json:"podName"`
	ContainerName string `json:"containerName"`
	Logs          string `json:"logs"`
}

// SaveLogsBundle saves multiple pod logs as a zip file
func (a *App) SaveLogsBundle(entries []PodLogEntry, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save Logs Bundle",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Zip Files (*.zip)",
				Pattern:     "*.zip",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User canceled
	}

	// Create the zip file
	zipFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, entry := range entries {
		// Create path: podName/containerName.log
		logPath := fmt.Sprintf("%s/%s.log", entry.PodName, entry.ContainerName)
		writer, err := zipWriter.Create(logPath)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", logPath, err)
		}
		_, err = writer.Write([]byte(entry.Logs))
		if err != nil {
			return fmt.Errorf("failed to write logs for %s: %w", logPath, err)
		}
	}

	return nil
}

// YamlBackupEntry represents a single resource's YAML for backup
type YamlBackupEntry struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Yaml      string `json:"yaml"`
}

// SaveYamlBackup saves multiple resource YAMLs as a zip file with native dialog
func (a *App) SaveYamlBackup(entries []YamlBackupEntry, defaultFilename string) error {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultFilename,
		Title:           "Save YAML Backup",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Zip Files (*.zip)",
				Pattern:     "*.zip",
			},
		},
	})

	if err != nil {
		return err
	}

	if filePath == "" {
		return nil // User canceled
	}

	// Create the zip file
	zipFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, entry := range entries {
		// Create path: namespace_name.yaml
		var yamlPath string
		if entry.Namespace != "" {
			yamlPath = fmt.Sprintf("%s_%s.yaml", entry.Namespace, entry.Name)
		} else {
			yamlPath = fmt.Sprintf("%s.yaml", entry.Name)
		}
		writer, err := zipWriter.Create(yamlPath)
		if err != nil {
			return fmt.Errorf("failed to create zip entry %s: %w", yamlPath, err)
		}
		_, err = writer.Write([]byte(entry.Yaml))
		if err != nil {
			return fmt.Errorf("failed to write YAML for %s: %w", yamlPath, err)
		}
	}

	return nil
}
