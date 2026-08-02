//go:build windows

package acceleratorrelease

import "os"

// Windows does not expose Unix mode bits. Cache files remain private through
// the user's config directory ACL; retain type and reparse-point rejection.
func privateCacheDirectory(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}

func privateCacheFile(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}
