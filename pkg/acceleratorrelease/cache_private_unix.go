//go:build !windows

package acceleratorrelease

import "os"

func privateCacheDirectory(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir() && info.Mode().Perm() == cacheDirectoryMode
}

func privateCacheFile(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() && info.Mode().Perm() == cacheFileMode
}
