//go:build windows

package acceleratorrelease

import "os"

func openPrivateCacheFile(path string) (cacheFile, error) { return os.Open(path) }
