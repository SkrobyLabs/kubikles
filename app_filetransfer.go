package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// ============================================================================
// Pod File Transfer Operations
// ============================================================================

// PodFileInfo represents a file or directory in a pod (re-exported for Wails binding)
type PodFileInfo struct {
	Name        string `json:"name"`
	IsDir       bool   `json:"isDir"`
	Size        int64  `json:"size"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	ModTime     string `json:"modTime"`
}

// ListPodFiles lists files in a directory inside a pod
func (a *App) ListPodFiles(namespace, pod, container, path string) ([]PodFileInfo, error) {
	debug.LogK8s("ListPodFiles called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "path": path})
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	files, err := a.k8sClient.ListFiles(context.Background(), namespace, pod, container, path)
	if err != nil {
		return nil, err
	}

	// Convert to PodFileInfo
	result := make([]PodFileInfo, len(files))
	for i, f := range files {
		result[i] = PodFileInfo{
			Name:        f.Name,
			IsDir:       f.IsDir,
			Size:        f.Size,
			Permissions: f.Permissions,
			Owner:       f.Owner,
			Group:       f.Group,
			ModTime:     f.ModTime,
		}
	}

	return result, nil
}

// DownloadPodFile downloads a file from a pod to local filesystem with save dialog
func (a *App) DownloadPodFile(namespace, pod, container, remotePath string) error {
	debug.LogK8s("DownloadPodFile called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "path": remotePath})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Get filename from path
	filename := remotePath
	if idx := len(remotePath) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if remotePath[i] == '/' {
				filename = remotePath[i+1:]
				break
			}
		}
	}

	// Open save dialog
	localPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: filename,
		Title:           "Save File",
	})
	if err != nil {
		return err
	}
	if localPath == "" {
		return nil // User canceled
	}

	// Get file size for progress
	size, _ := a.k8sClient.GetFileSize(context.Background(), namespace, pod, container, remotePath)

	// Emit initial progress
	a.emitEvent("file:progress", map[string]interface{}{
		"operation":        "download",
		"fileName":         filename,
		"bytesTransferred": 0,
		"totalBytes":       size,
		"done":             false,
	})

	// Download with progress callback
	err = a.k8sClient.DownloadFile(context.Background(), namespace, pod, container, remotePath, localPath, func(p k8s.FileProgress) {
		a.emitEvent("file:progress", map[string]interface{}{
			"operation":        p.Operation,
			"fileName":         p.FileName,
			"bytesTransferred": p.BytesTransferred,
			"totalBytes":       p.TotalBytes,
			"done":             p.Done,
			"error":            p.Error,
		})
	})

	if err != nil {
		a.emitEvent("file:progress", map[string]interface{}{
			"operation": "download",
			"fileName":  filename,
			"done":      true,
			"error":     err.Error(),
		})
		return err
	}

	return nil
}

// DownloadPodFolder downloads a folder from a pod as a tar.gz file
func (a *App) DownloadPodFolder(namespace, pod, container, remotePath string) error {
	debug.LogK8s("DownloadPodFolder called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "path": remotePath})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Get folder name from path
	folderName := path.Base(remotePath)
	if folderName == "" || folderName == "." || folderName == "/" {
		folderName = "root"
	}

	// Create safe filename for save dialog
	// macOS treats .app as special bundle extension which crashes the save dialog
	// Replace problematic extensions with underscores
	safeFilename := folderName
	for _, ext := range []string{".app", ".bundle", ".framework", ".plugin", ".kext"} {
		if strings.HasSuffix(strings.ToLower(safeFilename), ext) {
			safeFilename = safeFilename[:len(safeFilename)-len(ext)] + strings.ReplaceAll(ext, ".", "_")
			debug.LogK8s("DownloadPodFolder renamed to avoid macOS save dialog crash", map[string]interface{}{"original": folderName, "renamed": safeFilename})
		}
	}

	// Open save dialog
	// Note: On macOS, file filters can cause crashes with certain filenames
	// so we keep it simple with just the default filename
	localPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: safeFilename + ".tar.gz",
		Title:           "Save Folder as Archive",
	})
	if err != nil {
		return err
	}
	if localPath == "" {
		return nil // User canceled
	}

	// Emit initial progress
	a.emitEvent("file:progress", map[string]interface{}{
		"operation":        "download",
		"fileName":         folderName + ".tar.gz",
		"bytesTransferred": 0,
		"totalBytes":       int64(-1),
		"done":             false,
	})

	// Download with progress callback
	err = a.k8sClient.DownloadFolder(context.Background(), namespace, pod, container, remotePath, localPath, func(p k8s.FileProgress) {
		a.emitEvent("file:progress", map[string]interface{}{
			"operation":        p.Operation,
			"fileName":         p.FileName,
			"bytesTransferred": p.BytesTransferred,
			"totalBytes":       p.TotalBytes,
			"done":             p.Done,
			"error":            p.Error,
		})
	})

	if err != nil {
		a.emitEvent("file:progress", map[string]interface{}{
			"operation": "download",
			"fileName":  folderName + ".tar.gz",
			"done":      true,
			"error":     err.Error(),
		})
		return err
	}

	return nil
}

// DownloadPodFiles downloads multiple files/folders from a pod as a single tar.gz archive
func (a *App) DownloadPodFiles(namespace, pod, container, basePath string, names []string) error {
	debug.LogK8s("DownloadPodFiles called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "basePath": basePath, "count": len(names)})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Build filename: pod_container_unixtime.tar.gz
	archiveName := fmt.Sprintf("%s_%s_%d.tar.gz", pod, container, time.Now().Unix())

	// Open save dialog
	localPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: archiveName,
		Title:           "Save Files as Archive",
	})
	if err != nil {
		return err
	}
	if localPath == "" {
		return nil // User canceled
	}

	// Emit initial progress
	a.emitEvent("file:progress", map[string]interface{}{
		"operation":        "download",
		"fileName":         archiveName,
		"bytesTransferred": 0,
		"totalBytes":       int64(-1),
		"done":             false,
	})

	// Download with progress callback
	err = a.k8sClient.DownloadFiles(context.Background(), namespace, pod, container, basePath, names, localPath, func(p k8s.FileProgress) {
		a.emitEvent("file:progress", map[string]interface{}{
			"operation":        p.Operation,
			"fileName":         p.FileName,
			"bytesTransferred": p.BytesTransferred,
			"totalBytes":       p.TotalBytes,
			"done":             p.Done,
			"error":            p.Error,
		})
	})

	if err != nil {
		a.emitEvent("file:progress", map[string]interface{}{
			"operation": "download",
			"fileName":  archiveName,
			"done":      true,
			"error":     err.Error(),
		})
		return err
	}

	return nil
}

// UploadToPod uploads a file to a pod using file picker dialog
func (a *App) UploadToPod(namespace, pod, container, remotePath string) error {
	debug.LogK8s("UploadToPod called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "remotePath": remotePath})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Open file picker dialog
	localPath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select File to Upload",
	})
	if err != nil {
		return err
	}
	if localPath == "" {
		return nil // User canceled
	}

	return a.uploadFileInternal(namespace, pod, container, localPath, remotePath)
}

// UploadFileToPod uploads a file from a specific local path (for drag & drop)
func (a *App) UploadFileToPod(namespace, pod, container, localPath, remotePath string) error {
	debug.LogK8s("UploadFileToPod called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "localPath": localPath, "remotePath": remotePath})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	return a.uploadFileInternal(namespace, pod, container, localPath, remotePath)
}

// uploadFileInternal handles the actual file upload with progress
func (a *App) uploadFileInternal(namespace, pod, container, localPath, remotePath string) error {
	// Get file info
	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	filename := stat.Name()
	targetPath := remotePath
	if targetPath == "" || targetPath[len(targetPath)-1] == '/' {
		targetPath = targetPath + filename
	}

	// Emit initial progress
	a.emitEvent("file:progress", map[string]interface{}{
		"operation":        "upload",
		"fileName":         filename,
		"bytesTransferred": 0,
		"totalBytes":       stat.Size(),
		"done":             false,
	})

	var uploadErr error
	if stat.IsDir() {
		uploadErr = a.k8sClient.UploadFolder(context.Background(), namespace, pod, container, localPath, remotePath, func(p k8s.FileProgress) {
			a.emitEvent("file:progress", map[string]interface{}{
				"operation":        p.Operation,
				"fileName":         p.FileName,
				"bytesTransferred": p.BytesTransferred,
				"totalBytes":       p.TotalBytes,
				"done":             p.Done,
				"error":            p.Error,
			})
		})
	} else {
		uploadErr = a.k8sClient.UploadFile(context.Background(), namespace, pod, container, localPath, targetPath, func(p k8s.FileProgress) {
			a.emitEvent("file:progress", map[string]interface{}{
				"operation":        p.Operation,
				"fileName":         p.FileName,
				"bytesTransferred": p.BytesTransferred,
				"totalBytes":       p.TotalBytes,
				"done":             p.Done,
				"error":            p.Error,
			})
		})
	}

	if uploadErr != nil {
		a.emitEvent("file:progress", map[string]interface{}{
			"operation": "upload",
			"fileName":  filename,
			"done":      true,
			"error":     uploadErr.Error(),
		})
		return uploadErr
	}

	return nil
}

// CreatePodDirectory creates a directory in a pod
func (a *App) CreatePodDirectory(namespace, pod, container, dirPath string) error {
	debug.LogK8s("CreatePodDirectory called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "path": dirPath})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.CreateDirectory(context.Background(), namespace, pod, container, dirPath)
}

// DeletePodFile deletes a file or directory in a pod
func (a *App) DeletePodFile(namespace, pod, container, filePath string) error {
	debug.LogK8s("DeletePodFile called", map[string]interface{}{"namespace": namespace, "pod": pod, "container": container, "path": filePath})
	if a.k8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.DeleteFile(context.Background(), namespace, pod, container, filePath)
}
