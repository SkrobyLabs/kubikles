//go:build linux

package acceleratorrelease

import (
	"os"
	"syscall"
)

// O_NOFOLLOW makes the opened descriptor/marker the object we validate and
// read, instead of ever following a replacement symlink.
func openPrivateCacheFile(path string) (cacheFile, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
