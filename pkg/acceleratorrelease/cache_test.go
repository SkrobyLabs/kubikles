package acceleratorrelease

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type wrappedCacheFile struct {
	cacheFile
	writeErr error
	chmodErr error
	syncErr  error
	closeErr error
}

func (f *wrappedCacheFile) Chmod(mode os.FileMode) error {
	if f.chmodErr != nil {
		return f.chmodErr
	}
	return f.cacheFile.Chmod(mode)
}

func (f *wrappedCacheFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.cacheFile.Write(p)
}
func (f *wrappedCacheFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.cacheFile.Sync()
}
func (f *wrappedCacheFile) Close() error {
	err := f.cacheFile.Close()
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}

func TestCacheWriteIsAtomicAndPrivate(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	now := time.Unix(1_900_000_000, 0)
	t.Run("success", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		var events []string
		ops := osFileOps
		baseRename := ops.rename
		ops.rename = func(oldPath, newPath string) error {
			events = append(events, "rename:"+filepath.Ext(newPath))
			return baseRename(oldPath, newPath)
		}
		cache := newReleaseCache(root, ops)
		if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now); err != nil {
			t.Fatal(err)
		}
		descriptorPath, checksumPath := cachePaths(t, root, testBuildVersion, descriptorBytes)
		if filepath.Ext(descriptorPath) != ".json" || filepath.Ext(checksumPath) != ".sha256" || strings.HasSuffix(checksumPath, ".json.sha256") {
			t.Fatalf("cache names = %q, %q", descriptorPath, checksumPath)
		}
		rootInfo, _ := os.Stat(root)
		if rootInfo.Mode().Perm() != cacheDirectoryMode {
			t.Fatalf("root mode = %o", rootInfo.Mode().Perm())
		}
		for _, path := range []string{descriptorPath, checksumPath} {
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != cacheFileMode {
				t.Fatalf("unsafe cache file %s: %#v %v", path, info, err)
			}
		}
		if len(events) != 2 || events[0] != "rename:.json" || events[1] != "rename:.sha256" {
			t.Fatalf("rename order = %v", events)
		}
		if release, state := cache.loadExact(testBuildVersion, now); state != cacheOK || release.BuildVersion != testBuildVersion {
			t.Fatalf("load = %#v, %v", release, state)
		}
	})

	for _, point := range []string{
		"create-descriptor", "create-marker",
		"chmod-descriptor", "write-descriptor", "short-write", "file-sync-descriptor", "close-descriptor",
		"chmod-marker", "write-marker", "file-sync-marker", "close-marker",
		"rename-descriptor", "rename-marker", "chtimes-descriptor", "chtimes-marker",
		"directory-open", "directory-sync-post-descriptor", "directory-close-post-descriptor", "directory-sync-post-marker", "directory-close-post-marker",
	} {
		t.Run(point, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			ops := osFileOps
			failure := errors.New("injected " + point)
			createCount, renameCount, directorySyncCount := 0, 0, 0
			injected := false
			baseCreate := ops.createTmp
			ops.createTmp = func(directory, pattern string) (cacheFile, error) {
				createCount++
				if point == "create-descriptor" && createCount == 1 || point == "create-marker" && createCount == 2 {
					injected = true
					return nil, failure
				}
				file, err := baseCreate(directory, pattern)
				if err != nil {
					return nil, err
				}
				wrapped := &wrappedCacheFile{cacheFile: file}
				if createCount == 1 {
					switch point {
					case "chmod-descriptor":
						wrapped.chmodErr = failure
					case "write-descriptor":
						wrapped.writeErr = failure
					case "file-sync-descriptor":
						wrapped.syncErr = failure
					case "close-descriptor":
						wrapped.closeErr = failure
					}
				} else if createCount == 2 {
					switch point {
					case "chmod-marker":
						wrapped.chmodErr = failure
					case "write-marker":
						wrapped.writeErr = failure
					case "file-sync-marker":
						wrapped.syncErr = failure
					case "close-marker":
						wrapped.closeErr = failure
					}
				}
				if wrapped.chmodErr != nil || wrapped.writeErr != nil || wrapped.syncErr != nil || wrapped.closeErr != nil {
					injected = true
				}
				return wrapped, nil
			}
			if point == "short-write" {
				ops.createTmp = func(directory, pattern string) (cacheFile, error) {
					injected = true
					file, err := baseCreate(directory, pattern)
					if err != nil {
						return nil, err
					}
					return &shortWriteCacheFile{cacheFile: file}, nil
				}
			}
			baseRename := ops.rename
			ops.rename = func(oldPath, newPath string) error {
				renameCount++
				if point == "rename-descriptor" && renameCount == 1 || point == "rename-marker" && renameCount == 2 {
					injected = true
					return failure
				}
				return baseRename(oldPath, newPath)
			}
			baseChtimes := ops.chtimes
			ops.chtimes = func(path string, atime, mtime time.Time) error {
				if (point == "chtimes-descriptor" && strings.HasSuffix(path, ".json")) || (point == "chtimes-marker" && strings.HasSuffix(path, ".sha256")) {
					injected = true
					return failure
				}
				return baseChtimes(path, atime, mtime)
			}
			baseOpen := ops.open
			ops.open = func(path string) (cacheFile, error) {
				if point == "directory-open" {
					injected = true
					return nil, failure
				}
				file, err := baseOpen(path)
				if err != nil {
					return nil, err
				}
				directorySyncCount++
				if (directorySyncCount == 2 && (point == "directory-sync-post-descriptor" || point == "directory-close-post-descriptor")) ||
					(directorySyncCount == 3 && (point == "directory-sync-post-marker" || point == "directory-close-post-marker")) {
					wrapped := &wrappedCacheFile{cacheFile: file}
					if point == "directory-sync-post-descriptor" || point == "directory-sync-post-marker" {
						wrapped.syncErr = failure
					} else {
						wrapped.closeErr = failure
					}
					injected = true
					return wrapped, nil
				}
				return file, nil
			}
			cache := newReleaseCache(root, ops)
			if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now); err == nil {
				t.Fatal("fault was ignored")
			}
			if !injected {
				t.Fatal("fault point was not reached")
			}
			entries, err := os.ReadDir(root)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".tmp-") {
					t.Fatalf("temporary file leaked: %s", entry.Name())
				}
			}
			if _, state := newReleaseCache(root, osFileOps).loadExact(testBuildVersion, now); state == cacheOK {
				t.Fatal("failed write left a readable pair")
			}
		})
	}
}

type shortWriteCacheFile struct{ cacheFile }

func (f *shortWriteCacheFile) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestCacheConcurrentReadWrite(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	root := filepath.Join(t.TempDir(), "cache")
	cache := newReleaseCache(root, osFileOps)
	now := time.Unix(1_900_000_000, 0)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now); err != nil {
				t.Errorf("store: %v", err)
				return
			}
			if _, state := cache.loadExact(testBuildVersion, now); state != cacheOK {
				t.Errorf("load state = %v", state)
			}
		}()
	}
	wg.Wait()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %v, %v", entries, err)
	}
}

func TestCacheRevalidatesEveryRead(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	now := time.Unix(1_900_000_000, 0)
	mutations := map[string]func(string, string){
		"descriptor": func(descriptorPath, _ string) {
			_ = os.WriteFile(descriptorPath, append([]byte(nil), descriptorBytes[:len(descriptorBytes)-1]...), cacheFileMode)
		},
		"checksum": func(_, checksumPath string) {
			_ = os.WriteFile(checksumPath, []byte(strings.Repeat("0", 64)+"  other.json\n"), cacheFileMode)
		},
		"schema": func(descriptorPath, _ string) {
			b := bytes.Replace(descriptorBytes, []byte("release/accelerator-release.schema.json"), []byte("release/other.schema.json"), 1)
			_ = os.WriteFile(descriptorPath, b, cacheFileMode)
		},
		"reference": func(descriptorPath, _ string) {
			b := bytes.Replace(descriptorBytes, []byte("@sha256:cccc"), []byte(":v1.4.2cccc"), 1)
			_ = os.WriteFile(descriptorPath, b, cacheFileMode)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			cache := newReleaseCache(root, osFileOps)
			if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now); err != nil {
				t.Fatal(err)
			}
			descriptorPath, checksumPath := cachePaths(t, root, testBuildVersion, descriptorBytes)
			mutate(descriptorPath, checksumPath)
			if release, state := cache.loadExact(testBuildVersion, now); state != cacheBad || release != (VerifiedRelease{}) {
				t.Fatalf("mutation enabled cache: %#v %v", release, state)
			}
		})
	}
}

func TestCacheRejectsFutureMtime(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	now := time.Unix(1_900_000_000, 0)
	root := filepath.Join(t.TempDir(), "cache")
	writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now.Add(time.Nanosecond))
	if _, state := newReleaseCache(root, osFileOps).loadExact(testBuildVersion, now); state != cacheBad {
		t.Fatalf("future cache state = %v", state)
	}
}

func TestCacheRejectsUnsafeEntries(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	now := time.Unix(1_900_000_000, 0)
	tests := map[string]func(*testing.T, string){
		"symlink": func(t *testing.T, root string) {
			descriptorPath, checksumPath := cachePaths(t, root, testBuildVersion, descriptorBytes)
			outside := filepath.Join(t.TempDir(), "outside")
			_ = os.WriteFile(outside, descriptorBytes, cacheFileMode)
			if err := os.Symlink(outside, descriptorPath); err != nil {
				t.Fatal(err)
			}
			_ = os.WriteFile(checksumPath, checksumBytes, cacheFileMode)
		},
		"non-regular": func(t *testing.T, root string) {
			descriptorPath, checksumPath := cachePaths(t, root, testBuildVersion, descriptorBytes)
			_ = os.Mkdir(descriptorPath, cacheFileMode)
			_ = os.WriteFile(checksumPath, checksumBytes, cacheFileMode)
		},
		"oversize descriptor": func(t *testing.T, root string) {
			descriptorPath, checksumPath := cachePaths(t, root, testBuildVersion, descriptorBytes)
			_ = os.WriteFile(descriptorPath, make([]byte, maxDescriptorBytes+1), cacheFileMode)
			_ = os.WriteFile(checksumPath, checksumBytes, cacheFileMode)
		},
		"oversize marker": func(t *testing.T, root string) {
			descriptorPath, checksumPath := cachePaths(t, root, testBuildVersion, descriptorBytes)
			_ = os.WriteFile(descriptorPath, descriptorBytes, cacheFileMode)
			_ = os.WriteFile(checksumPath, make([]byte, maxChecksumBytes+1), cacheFileMode)
		},
		"unknown": func(t *testing.T, root string) {
			writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now)
			_ = os.WriteFile(filepath.Join(root, "unknown"), []byte("x"), cacheFileMode)
		},
		"partial": func(t *testing.T, root string) {
			descriptorPath, _ := cachePaths(t, root, testBuildVersion, descriptorBytes)
			_ = os.WriteFile(descriptorPath, descriptorBytes, cacheFileMode)
		},
		"public mode": func(t *testing.T, root string) {
			descriptorPath, _ := writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now)
			_ = os.Chmod(descriptorPath, 0644)
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			if err := os.MkdirAll(root, cacheDirectoryMode); err != nil {
				t.Fatal(err)
			}
			setup(t, root)
			if release, state := newReleaseCache(root, osFileOps).loadExact(testBuildVersion, now); state != cacheBad || release != (VerifiedRelease{}) {
				t.Fatalf("unsafe entry accepted: %#v %v", release, state)
			}
		})
	}
	t.Run("other version", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		otherDescriptor, otherChecksum := pairForVersion(t, "v1.4.3")
		writePairDirect(t, root, "v1.4.3", otherDescriptor, otherChecksum, now)
		if _, state := newReleaseCache(root, osFileOps).loadExact(testBuildVersion, now); state != cacheNone {
			t.Fatalf("state = %v", state)
		}
	})
	t.Run("read failure", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now)
		ops := osFileOps
		ops.open = func(string) (cacheFile, error) { return nil, errors.New("injected read failure") }
		if release, state := newReleaseCache(root, ops).loadExact(testBuildVersion, now); state != cacheIO || release != (VerifiedRelease{}) {
			t.Fatalf("read failure = %#v %v", release, state)
		}
	})
	t.Run("filename digest binding", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		descriptorPath, checksumPath := writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now)
		wrongStem := filepath.Join(root, testBuildVersion+"--"+strings.Repeat("0", 64))
		if err := os.Rename(descriptorPath, wrongStem+".json"); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(checksumPath, wrongStem+".sha256"); err != nil {
			t.Fatal(err)
		}
		if release, state := newReleaseCache(root, osFileOps).loadExact(testBuildVersion, now); state != cacheBad || release != (VerifiedRelease{}) {
			t.Fatalf("wrong filename digest = %#v %v", release, state)
		}
	})
	t.Run("conflict", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now)
		var wire descriptor
		_ = json.Unmarshal(descriptorBytes, &wire)
		wire.Source.Commit = strings.Repeat("d", 40)
		wire.Schema = "https://raw.githubusercontent.com/SkrobyLabs/kubikles/" + wire.Source.Commit + "/release/accelerator-release.schema.json"
		otherDescriptor, otherChecksum := encodeTestPair(t, testBuildVersion, wire)
		writePairDirect(t, root, testBuildVersion, otherDescriptor, otherChecksum, now)
		cache := newReleaseCache(root, osFileOps)
		if _, state := cache.loadExact(testBuildVersion, now); state != cacheBad {
			t.Fatalf("state = %v", state)
		}
		entriesBefore, _ := os.ReadDir(root)
		if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now.Add(time.Hour)); !errors.Is(err, os.ErrExist) {
			t.Fatalf("conflicting store error = %v", err)
		}
		entriesAfter, _ := os.ReadDir(root)
		if len(entriesAfter) != len(entriesBefore) {
			t.Fatalf("conflicting store mutated entries: %d -> %d", len(entriesBefore), len(entriesAfter))
		}
	})
	t.Run("age boundary", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			age  time.Duration
			want cacheState
		}{{"exact", cacheMaxAge, cacheOK}, {"expired", cacheMaxAge + time.Nanosecond, cacheBad}} {
			t.Run(tc.name, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "cache")
				writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now.Add(-tc.age))
				if _, state := newReleaseCache(root, osFileOps).loadExact(testBuildVersion, now); state != tc.want {
					t.Fatalf("state = %v", state)
				}
			})
		}
	})
}

func TestCacheEvictsDeterministically(t *testing.T) {
	for _, sameTime := range []bool{false, true} {
		t.Run(map[bool]string{false: "oldest", true: "lexical tie"}[sameTime], func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			cache := newReleaseCache(root, osFileOps)
			base := time.Unix(1_900_000_000, 0)
			versions := []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "v1.4.0", "v1.5.0"}
			for i, version := range versions {
				descriptorBytes, checksumBytes := pairForVersion(t, version)
				when := base
				if !sameTime {
					when = base.Add(time.Duration(i) * time.Hour)
				}
				if err := cache.storeVerified(version, checksumBytes, descriptorBytes, when); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			sort.Strings(names)
			if len(names) != cacheMaxPairs*2 {
				t.Fatalf("entry count = %d: %v", len(names), names)
			}
			for _, name := range names {
				if strings.HasPrefix(name, "v1.0.0--") {
					t.Fatalf("oldest/lexical entry survived: %s", name)
				}
				if strings.HasPrefix(name, ".tmp-") {
					t.Fatalf("temp survived: %s", name)
				}
			}
			lastDescriptor, _ := pairForVersion(t, "v1.5.0")
			descriptorPath, checksumPath := cachePaths(t, root, "v1.5.0", lastDescriptor)
			if _, err := os.Stat(descriptorPath); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(checksumPath); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("cleans stale partials", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		_ = os.MkdirAll(root, cacheDirectoryMode)
		_ = os.WriteFile(filepath.Join(root, ".tmp-stale"), []byte("x"), cacheFileMode)
		partialDescriptor, _ := pairForVersion(t, "v2.0.0")
		partialPath, _ := cachePaths(t, root, "v2.0.0", partialDescriptor)
		_ = os.WriteFile(partialPath, partialDescriptor, cacheFileMode)
		descriptorBytes, checksumBytes := goldenPair(t)
		if err := newReleaseCache(root, osFileOps).storeVerified(testBuildVersion, checksumBytes, descriptorBytes, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, ".tmp-stale")); !os.IsNotExist(err) {
			t.Fatalf("temp remains: %v", err)
		}
		if _, err := os.Stat(partialPath); !os.IsNotExist(err) {
			t.Fatalf("partial remains: %v", err)
		}
	})
	t.Run("refreshes exact pair", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		cache := newReleaseCache(root, osFileOps)
		descriptorBytes, checksumBytes := goldenPair(t)
		first := time.Unix(1_900_000_000, 0)
		second := first.Add(time.Hour)
		if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, first); err != nil {
			t.Fatal(err)
		}
		if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, second); err != nil {
			t.Fatal(err)
		}
		descriptorPath, checksumPath := cachePaths(t, root, testBuildVersion, descriptorBytes)
		for _, path := range []string{descriptorPath, checksumPath} {
			info, err := os.Stat(path)
			if err != nil || !info.ModTime().Equal(second) {
				t.Fatalf("%s refresh time = %v, %v", path, info, err)
			}
		}
		entries, _ := os.ReadDir(root)
		if len(entries) != 2 {
			t.Fatalf("refresh created extra entries: %v", entries)
		}
	})
}

var _ = io.ErrShortWrite
