package k8s

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	// EphemeralContainerName is the name used for the file helper ephemeral container
	EphemeralContainerName = "kk-file-helper"
	// EphemeralContainerImage is the image used for the ephemeral container
	EphemeralContainerImage = "busybox:stable"
)

// FileInfo represents a file or directory in a pod
type FileInfo struct {
	Name        string `json:"name"`
	IsDir       bool   `json:"isDir"`
	Size        int64  `json:"size"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	ModTime     string `json:"modTime"`
}

// FileProgress represents progress of a file transfer operation
type FileProgress struct {
	Operation        string `json:"operation"` // "upload" or "download"
	FileName         string `json:"fileName"`
	BytesTransferred int64  `json:"bytesTransferred"`
	TotalBytes       int64  `json:"totalBytes"` // -1 if unknown
	Done             bool   `json:"done"`
	Error            string `json:"error,omitempty"`
}

// ProgressCallback is called during file transfers to report progress
type ProgressCallback func(progress FileProgress)

// ListFiles lists files in a directory inside a pod
func (c *Client) ListFiles(ctx context.Context, namespace, pod, container, dirPath string) ([]FileInfo, error) {
	if dirPath == "" {
		dirPath = "/"
	}

	// Try to get or create a file helper container if needed
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	// Use ls with consistent output format
	// -l: long format, -a: all files, -L: follow symlinks for type detection
	// --time-style=long-iso: consistent time format
	// We avoid -h (human readable) to get exact byte sizes
	cmd := []string{"ls", "-laL", "--time-style=long-iso", dirPath}

	stdout, _, err := c.execInPod(ctx, namespace, pod, effectiveContainer, cmd)
	if err != nil {
		// Try without -L (some systems don't support it or have broken symlinks)
		cmd = []string{"ls", "-la", "--time-style=long-iso", dirPath}
		stdout, _, err = c.execInPod(ctx, namespace, pod, effectiveContainer, cmd)
		if err != nil {
			// Final fallback: basic ls
			cmd = []string{"ls", "-la", dirPath}
			var stderr string
			stdout, stderr, err = c.execInPod(ctx, namespace, pod, effectiveContainer, cmd)
			if err != nil {
				return nil, fmt.Errorf("failed to list files: %w, stderr: %s", err, stderr)
			}
		}
	}

	return parseListOutput(stdout), nil
}

// parseListOutput parses ls -la output into FileInfo structs
func parseListOutput(output string) []FileInfo {
	var files []FileInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}

		fi := parseLsLine(line)
		if fi != nil && fi.Name != "." {
			files = append(files, *fi)
		}
	}

	// Sort: directories first, then alphabetically by name
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files
}

// parseLsLine parses a single line from ls -la output
func parseLsLine(line string) *FileInfo {
	// ls -la output format:
	// -rw-r--r-- 1 root root 1234 2024-01-15 10:30 filename
	// drwxr-xr-x 2 root root 4096 2024-01-15 10:30 dirname
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return nil
	}

	perms := fields[0]
	owner := fields[2]
	group := fields[3]

	// Size is typically field 4, but could vary
	sizeStr := fields[4]
	size, _ := strconv.ParseInt(sizeStr, 10, 64)

	// Find the filename - it's everything after the timestamp
	// Look for date pattern (YYYY-MM-DD or Mon DD or similar)
	nameStartIdx := 0
	for i := 5; i < len(fields); i++ {
		// Check if this looks like a time (HH:MM)
		if strings.Contains(fields[i], ":") && len(fields[i]) <= 5 {
			nameStartIdx = i + 1
			break
		}
		// Check for year (4 digits)
		if len(fields[i]) == 4 {
			if _, err := strconv.Atoi(fields[i]); err == nil {
				nameStartIdx = i + 1
				break
			}
		}
	}

	if nameStartIdx == 0 || nameStartIdx >= len(fields) {
		// Fallback: assume standard format with date and time
		if len(fields) >= 9 {
			nameStartIdx = 8
		} else {
			return nil
		}
	}

	name := strings.Join(fields[nameStartIdx:], " ")
	// Handle symlinks: "name -> target"
	if idx := strings.Index(name, " -> "); idx != -1 {
		name = name[:idx]
	}

	// Build modTime from available fields
	modTime := ""
	if nameStartIdx >= 2 {
		modTime = strings.Join(fields[5:nameStartIdx], " ")
	}

	return &FileInfo{
		Name:        name,
		IsDir:       perms[0] == 'd',
		Size:        size,
		Permissions: perms,
		Owner:       owner,
		Group:       group,
		ModTime:     modTime,
	}
}

// GetFileSize gets the size of a file in a pod
func (c *Client) GetFileSize(ctx context.Context, namespace, pod, container, filePath string) (int64, error) {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	cmd := []string{"stat", "-c", "%s", filePath}
	stdout, _, err := c.execInPod(ctx, namespace, pod, effectiveContainer, cmd)
	if err != nil {
		// Try macOS/BSD stat format
		cmd = []string{"stat", "-f", "%z", filePath}
		var stderr string
		stdout, stderr, err = c.execInPod(ctx, namespace, pod, effectiveContainer, cmd)
		if err != nil {
			return -1, fmt.Errorf("failed to get file size: %w, stderr: %s", err, stderr)
		}
	}

	size, err := strconv.ParseInt(strings.TrimSpace(stdout), 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse file size: %w", err)
	}

	return size, nil
}

// DownloadFile downloads a file from a pod to local filesystem
func (c *Client) DownloadFile(ctx context.Context, namespace, pod, container, remotePath, localPath string, progress ProgressCallback) error {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	// Get file size first for progress reporting
	totalSize, _ := c.GetFileSize(ctx, namespace, pod, effectiveContainer, remotePath)

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// Use cat for single files
	cmd := []string{"cat", remotePath}

	// Create progress writer
	pw := &progressWriter{
		writer:    localFile,
		fileName:  path.Base(remotePath),
		totalSize: totalSize,
		callback:  progress,
		operation: "download",
	}

	err = c.execInPodStream(ctx, namespace, pod, effectiveContainer, cmd, nil, pw)
	if err != nil {
		os.Remove(localPath)
		return fmt.Errorf("failed to download file: %w", err)
	}

	// Report completion
	if progress != nil {
		progress(FileProgress{
			Operation:        "download",
			FileName:         path.Base(remotePath),
			BytesTransferred: pw.written,
			TotalBytes:       totalSize,
			Done:             true,
		})
	}

	return nil
}

// DownloadFolder downloads a folder from a pod as a tar.gz file
func (c *Client) DownloadFolder(ctx context.Context, namespace, pod, container, remotePath, localPath string, progress ProgressCallback) error {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// Use tar to archive the folder, piped through gzip
	// We cd to parent dir and tar the basename to avoid absolute paths in archive
	parentDir := path.Dir(remotePath)
	baseName := path.Base(remotePath)
	cmd := []string{"sh", "-c", fmt.Sprintf("cd %s && tar cf - %s | gzip", parentDir, baseName)}

	pw := &progressWriter{
		writer:    localFile,
		fileName:  baseName + ".tar.gz",
		totalSize: -1, // Unknown for folders
		callback:  progress,
		operation: "download",
	}

	err = c.execInPodStream(ctx, namespace, pod, effectiveContainer, cmd, nil, pw)
	if err != nil {
		os.Remove(localPath)
		return fmt.Errorf("failed to download folder: %w", err)
	}

	if progress != nil {
		progress(FileProgress{
			Operation:        "download",
			FileName:         baseName + ".tar.gz",
			BytesTransferred: pw.written,
			TotalBytes:       pw.written,
			Done:             true,
		})
	}

	return nil
}

// DownloadFiles downloads multiple files/folders from a pod as a single tar.gz archive
func (c *Client) DownloadFiles(ctx context.Context, namespace, pod, container, basePath string, names []string, localPath string, progress ProgressCallback) error {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// Build shell-quoted list of names
	var quoted []string
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(n, "'", "'\\''")))
	}
	cmd := []string{"sh", "-c", fmt.Sprintf("cd %s && tar cf - %s | gzip", basePath, strings.Join(quoted, " "))}

	archiveName := path.Base(localPath)
	pw := &progressWriter{
		writer:    localFile,
		fileName:  archiveName,
		totalSize: -1, // Unknown for multi-file archive
		callback:  progress,
		operation: "download",
	}

	err = c.execInPodStream(ctx, namespace, pod, effectiveContainer, cmd, nil, pw)
	if err != nil {
		os.Remove(localPath)
		return fmt.Errorf("failed to download files: %w", err)
	}

	if progress != nil {
		progress(FileProgress{
			Operation:        "download",
			FileName:         archiveName,
			BytesTransferred: pw.written,
			TotalBytes:       pw.written,
			Done:             true,
		})
	}

	return nil
}

// UploadFile uploads a local file to a pod
func (c *Client) UploadFile(ctx context.Context, namespace, pod, container, localPath, remotePath string, progress ProgressCallback) error {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Get file info for size
	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	// Create tar archive in memory with single file
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	hdr := &tar.Header{
		Name: path.Base(remotePath),
		Mode: 0644,
		Size: stat.Size(),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := io.Copy(tw, localFile); err != nil {
		return fmt.Errorf("failed to write file to tar: %w", err)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Extract tar in the target directory
	targetDir := path.Dir(remotePath)
	cmd := []string{"tar", "xf", "-", "-C", targetDir}

	// Create progress reader
	pr := &progressReader{
		reader:    &tarBuf,
		fileName:  path.Base(localPath),
		totalSize: int64(tarBuf.Len()),
		callback:  progress,
		operation: "upload",
	}

	err = c.execInPodStream(ctx, namespace, pod, effectiveContainer, cmd, pr, io.Discard)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	if progress != nil {
		progress(FileProgress{
			Operation:        "upload",
			FileName:         path.Base(localPath),
			BytesTransferred: stat.Size(),
			TotalBytes:       stat.Size(),
			Done:             true,
		})
	}

	return nil
}

// UploadFolder uploads a local folder to a pod as tar
func (c *Client) UploadFolder(ctx context.Context, namespace, pod, container, localPath, remotePath string, progress ProgressCallback) error {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	// Create tar.gz of the local folder
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	basePath := path.Base(localPath)

	err := addToTar(tw, localPath, basePath)
	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}

	tw.Close()
	gw.Close()

	// Extract in target directory
	cmd := []string{"sh", "-c", fmt.Sprintf("cd %s && gunzip | tar xf -", remotePath)}

	pr := &progressReader{
		reader:    &buf,
		fileName:  basePath,
		totalSize: int64(buf.Len()),
		callback:  progress,
		operation: "upload",
	}

	err = c.execInPodStream(ctx, namespace, pod, effectiveContainer, cmd, pr, io.Discard)
	if err != nil {
		return fmt.Errorf("failed to upload folder: %w", err)
	}

	if progress != nil {
		progress(FileProgress{
			Operation:        "upload",
			FileName:         basePath,
			BytesTransferred: int64(buf.Len()),
			TotalBytes:       int64(buf.Len()),
			Done:             true,
		})
	}

	return nil
}

// CreateDirectory creates a directory in a pod
func (c *Client) CreateDirectory(ctx context.Context, namespace, pod, container, dirPath string) error {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	cmd := []string{"mkdir", "-p", dirPath}
	_, stderr, err := c.execInPod(ctx, namespace, pod, effectiveContainer, cmd)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w, stderr: %s", err, stderr)
	}
	return nil
}

// DeleteFile deletes a file or directory in a pod
func (c *Client) DeleteFile(ctx context.Context, namespace, pod, container, filePath string) error {
	effectiveContainer, _ := c.GetOrCreateFileHelper(ctx, namespace, pod, container)

	cmd := []string{"rm", "-rf", filePath}
	_, stderr, err := c.execInPod(ctx, namespace, pod, effectiveContainer, cmd)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w, stderr: %s", err, stderr)
	}
	return nil
}

// execInPod executes a command in a pod and returns stdout/stderr
func (c *Client) execInPod(ctx context.Context, namespace, pod, container string, cmd []string) (string, string, error) {
	var stdout, stderr bytes.Buffer

	err := c.execInPodStream(ctx, namespace, pod, container, cmd, nil, &stdout)
	if err != nil {
		return "", stderr.String(), err
	}

	return stdout.String(), stderr.String(), nil
}

// ExecCommandInPod runs cmd in the given pod/container and returns stdout output.
func (c *Client) ExecCommandInPod(namespace, pod, container string, cmd []string) (string, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	stdout, stderr, err := c.execInPod(ctx, namespace, pod, container, cmd)
	if err != nil {
		if stderr != "" {
			return "", fmt.Errorf("%w: %s", err, stderr)
		}
		return "", err
	}
	return stdout, nil
}

// execInPodStream executes a command in a pod with streaming I/O
func (c *Client) execInPodStream(ctx context.Context, namespace, pod, container string, cmd []string, stdin io.Reader, stdout io.Writer) error {
	// Get REST config from the client config
	restConfig, err := c.configLoading.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get REST config: %w", err)
	}

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   cmd,
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	var stderrBuf bytes.Buffer

	streamOpts := remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: &stderrBuf,
		Tty:    false,
	}

	if stdin != nil {
		streamOpts.Stdin = stdin
	}

	err = exec.StreamWithContext(ctx, streamOpts)
	if err != nil {
		stderrStr := stderrBuf.String()
		if stderrStr != "" {
			return fmt.Errorf("%w: %s", err, stderrStr)
		}
		return err
	}

	return nil
}

// checkToolsAvailable checks if required tools (ls, cat, tar) are available in the container
func (c *Client) checkToolsAvailable(ctx context.Context, namespace, pod, container string) bool {
	// Try a simple command that uses the tools we need
	cmd := []string{"sh", "-c", "command -v ls && command -v cat && command -v tar"}
	_, _, err := c.execInPod(ctx, namespace, pod, container, cmd)
	return err == nil
}

// hasEphemeralContainer checks if the ephemeral container already exists
func (c *Client) hasEphemeralContainer(ctx context.Context, namespace, podName string) (bool, bool, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return false, false, err
	}

	for _, ec := range pod.Spec.EphemeralContainers {
		if ec.Name == EphemeralContainerName {
			// Check if it's running
			for _, status := range pod.Status.EphemeralContainerStatuses {
				if status.Name == EphemeralContainerName {
					if status.State.Running != nil {
						return true, true, nil
					}
					return true, false, nil
				}
			}
			return true, false, nil
		}
	}
	return false, false, nil
}

// injectEphemeralContainer adds an ephemeral container with file tools to the pod
func (c *Client) injectEphemeralContainer(ctx context.Context, namespace, podName, targetContainer string) error {
	// Get the pod to find volume mounts from target container
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	// Find the target container to copy its volume mounts
	var volumeMounts []corev1.VolumeMount
	for _, c := range pod.Spec.Containers {
		if c.Name == targetContainer {
			volumeMounts = c.VolumeMounts
			break
		}
	}

	// Create the ephemeral container spec
	ec := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:            EphemeralContainerName,
			Image:           EphemeralContainerImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"sh", "-c", "sleep 3600"},
			VolumeMounts:    volumeMounts,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  pod.Spec.SecurityContext.RunAsUser,
				RunAsGroup: pod.Spec.SecurityContext.RunAsGroup,
			},
		},
		TargetContainerName: targetContainer,
	}

	// Clear security context if pod doesn't have one
	if pod.Spec.SecurityContext == nil {
		ec.SecurityContext = nil
	}

	// Patch the pod to add the ephemeral container
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"ephemeralContainers": append(pod.Spec.EphemeralContainers, ec),
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = c.clientset.CoreV1().Pods(namespace).Patch(
		ctx,
		podName,
		types.StrategicMergePatchType,
		patchBytes,
		metav1.PatchOptions{},
		"ephemeralcontainers",
	)
	if err != nil {
		return fmt.Errorf("failed to inject ephemeral container: %w", err)
	}

	return nil
}

// waitForEphemeralContainer waits for the ephemeral container to be running
func (c *Client) waitForEphemeralContainer(ctx context.Context, namespace, podName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		_, running, err := c.hasEphemeralContainer(ctx, namespace, podName)
		if err != nil {
			return err
		}
		if running {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for ephemeral container to start")
}

// GetOrCreateFileHelper ensures a container with file tools is available
// Returns the container name to use for file operations
func (c *Client) GetOrCreateFileHelper(ctx context.Context, namespace, podName, preferredContainer string) (string, error) {
	// First, check if the preferred container has the required tools
	if c.checkToolsAvailable(ctx, namespace, podName, preferredContainer) {
		return preferredContainer, nil
	}

	// Check if we already have an ephemeral container
	exists, running, err := c.hasEphemeralContainer(ctx, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("failed to check ephemeral container: %w", err)
	}

	if exists && running {
		return EphemeralContainerName, nil
	}

	if !exists {
		// Inject the ephemeral container
		if err := c.injectEphemeralContainer(ctx, namespace, podName, preferredContainer); err != nil {
			// If injection fails (e.g., no RBAC or feature not supported), fall back to preferred container
			// The operation might still work if basic tools exist
			return preferredContainer, nil //nolint:nilerr // intentional fallback
		}
	}

	// Wait for it to be ready
	if err := c.waitForEphemeralContainer(ctx, namespace, podName, 30*time.Second); err != nil {
		return preferredContainer, nil //nolint:nilerr // intentional fallback on timeout
	}

	return EphemeralContainerName, nil
}

// progressWriter wraps a writer to track progress
type progressWriter struct {
	writer    io.Writer
	written   int64
	totalSize int64
	fileName  string
	callback  ProgressCallback
	operation string
	lastEmit  int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.written += int64(n)

	// Emit progress every 64KB or on completion
	if pw.callback != nil && (pw.written-pw.lastEmit >= 65536 || err != nil) {
		pw.callback(FileProgress{
			Operation:        pw.operation,
			FileName:         pw.fileName,
			BytesTransferred: pw.written,
			TotalBytes:       pw.totalSize,
			Done:             false,
		})
		pw.lastEmit = pw.written
	}

	return n, err
}

// progressReader wraps a reader to track progress
type progressReader struct {
	reader    io.Reader
	read      int64
	totalSize int64
	fileName  string
	callback  ProgressCallback
	operation string
	lastEmit  int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)

	// Emit progress every 64KB
	if pr.callback != nil && (pr.read-pr.lastEmit >= 65536 || err == io.EOF) {
		pr.callback(FileProgress{
			Operation:        pr.operation,
			FileName:         pr.fileName,
			BytesTransferred: pr.read,
			TotalBytes:       pr.totalSize,
			Done:             false,
		})
		pr.lastEmit = pr.read
	}

	return n, err
}

// addToTar recursively adds files to a tar archive
func addToTar(tw *tar.Writer, srcPath, basePath string) error {
	return addToTarWalk(tw, srcPath, basePath)
}

func addToTarWalk(tw *tar.Writer, srcPath, basePath string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		entries, err := os.ReadDir(srcPath)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			childPath := path.Join(srcPath, entry.Name())
			childBase := path.Join(basePath, entry.Name())
			if err := addToTarWalk(tw, childPath, childBase); err != nil {
				return err
			}
		}
		return nil
	}

	// Regular file
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	hdr := &tar.Header{
		Name: basePath,
		Mode: int64(info.Mode()),
		Size: info.Size(),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	return err
}
