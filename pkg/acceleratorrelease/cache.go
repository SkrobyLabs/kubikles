package acceleratorrelease

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	cacheDirectoryMode = 0700
	cacheFileMode      = 0600
	cacheMaxPairs      = 5
	cacheMaxAge        = 30 * 24 * time.Hour
)

var cacheNameRE = regexp.MustCompile(`^(v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?)--([0-9a-f]{64})(\.json|\.sha256)$`)

type cacheState int

const (
	cacheOK cacheState = iota
	cacheNone
	cacheBad
	cacheIO
)

type cacheFile interface {
	io.Reader
	io.Writer
	Chmod(os.FileMode) error
	Sync() error
	Close() error
	Name() string
	Stat() (os.FileInfo, error)
}

type fileOps struct {
	mkdirAll func(string, os.FileMode) error
	chmod    func(string, os.FileMode) error
	readDir  func(string) ([]os.DirEntry, error)
	lstat    func(string) (os.FileInfo, error)
	// retained for narrow legacy fault tests; cache reads use open+Stat+bounded Read.
	readFile  func(string) ([]byte, error)
	createTmp func(string, string) (cacheFile, error)
	rename    func(string, string) error
	remove    func(string) error
	open      func(string) (cacheFile, error)
	chtimes   func(string, time.Time, time.Time) error
}

var osFileOps = fileOps{
	mkdirAll: os.MkdirAll,
	chmod:    os.Chmod,
	readDir:  os.ReadDir,
	lstat:    os.Lstat,
	readFile: os.ReadFile,
	createTmp: func(directory, pattern string) (cacheFile, error) {
		return os.CreateTemp(directory, pattern)
	},
	rename:  os.Rename,
	remove:  os.Remove,
	open:    openPrivateCacheFile,
	chtimes: os.Chtimes,
}

type releaseCache struct {
	root string
	ops  fileOps
	mu   sync.Mutex
}

func newReleaseCache(root string, ops fileOps) *releaseCache {
	return &releaseCache{root: root, ops: ops}
}

type cachePair struct {
	version        string
	digest         string
	descriptorName string
	checksumName   string
}

func (c *releaseCache) loadExact(buildVersion string, now time.Time) (VerifiedRelease, cacheState) {
	return c.loadExactDigest(buildVersion, "", now)
}

func (c *releaseCache) loadExactDigest(buildVersion, requiredDigest string, now time.Time) (VerifiedRelease, cacheState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !releaseVersion.MatchString(buildVersion) {
		return VerifiedRelease{}, cacheBad
	}
	rootInfo, err := c.ops.lstat(c.root)
	if errors.Is(err, os.ErrNotExist) {
		return VerifiedRelease{}, cacheNone
	}
	if err != nil {
		return VerifiedRelease{}, cacheIO
	}
	if !privateCacheDirectory(rootInfo) {
		return VerifiedRelease{}, cacheBad
	}
	entries, err := c.ops.readDir(c.root)
	if err != nil {
		return VerifiedRelease{}, cacheIO
	}
	pairs, unsafe := collectCachePairs(entries)
	if unsafe {
		return VerifiedRelease{}, cacheBad
	}
	var exact []cachePair
	for _, pair := range pairs {
		if pair.version == buildVersion {
			exact = append(exact, pair)
		}
	}
	if len(exact) == 0 {
		return VerifiedRelease{}, cacheNone
	}
	if len(exact) != 1 || exact[0].descriptorName == "" || exact[0].checksumName == "" {
		return VerifiedRelease{}, cacheBad
	}
	pair := exact[0]
	if requiredDigest != "" && pair.digest != requiredDigest {
		return VerifiedRelease{}, cacheBad
	}
	descriptorPath := filepath.Join(c.root, pair.descriptorName)
	checksumPath := filepath.Join(c.root, pair.checksumName)
	descriptorBytes, state := c.readCacheFile(descriptorPath, maxDescriptorBytes, now)
	if state != cacheOK {
		return VerifiedRelease{}, state
	}
	checksumBytes, state := c.readCacheFile(checksumPath, maxChecksumBytes, now)
	if state != cacheOK {
		return VerifiedRelease{}, state
	}
	release, class := validateAndProject(buildVersion, checksumBytes, descriptorBytes)
	if class != descriptorValid || release.DescriptorSHA256 != pair.digest {
		return VerifiedRelease{}, cacheBad
	}
	return release, cacheOK
}

// readCacheFile validates the same opened object it reads. The Lstat identity
// comparison detects replacement between directory scan and open.
func (c *releaseCache) readCacheFile(path string, limit int, now time.Time) ([]byte, cacheState) {
	before, err := c.ops.lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, cacheBad
		}
		return nil, cacheIO
	}
	if !privateCacheFile(before) || staleCacheTime(before.ModTime(), now) {
		return nil, cacheBad
	}
	f, err := c.ops.open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, cacheBad
		}
		return nil, cacheIO
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, cacheIO
	}
	if !privateCacheFile(info) || staleCacheTime(info.ModTime(), now) || !os.SameFile(before, info) {
		return nil, cacheBad
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, cacheIO
	}
	if len(b) > limit {
		return nil, cacheBad
	}
	return b, cacheOK
}

func (c *releaseCache) safeCacheFileInfo(path string, limit int, now time.Time) (os.FileInfo, cacheState) {
	info, err := c.ops.lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, cacheBad
		}
		return nil, cacheIO
	}
	if !privateCacheFile(info) || info.Size() < 0 || info.Size() > int64(limit) || staleCacheTime(info.ModTime(), now) {
		return nil, cacheBad
	}
	return info, cacheOK
}

func staleCacheTime(modTime, now time.Time) bool {
	return modTime.After(now) || now.Sub(modTime) > cacheMaxAge
}

func collectCachePairs(entries []os.DirEntry) ([]cachePair, bool) {
	byKey := make(map[string]*cachePair)
	unsafe := false
	for _, entry := range entries {
		match := cacheNameRE.FindStringSubmatch(entry.Name())
		if match == nil {
			unsafe = true
			continue
		}
		key := match[1] + "--" + match[2]
		pair := byKey[key]
		if pair == nil {
			pair = &cachePair{version: match[1], digest: match[2]}
			byKey[key] = pair
		}
		if match[3] == ".json" {
			if pair.descriptorName != "" {
				unsafe = true
			}
			pair.descriptorName = entry.Name()
		} else {
			if pair.checksumName != "" {
				unsafe = true
			}
			pair.checksumName = entry.Name()
		}
	}
	pairs := make([]cachePair, 0, len(byKey))
	for _, pair := range byKey {
		pairs = append(pairs, *pair)
	}
	return pairs, unsafe
}

func (c *releaseCache) storeVerified(buildVersion string, checksumBytes, descriptorBytes []byte, now time.Time) error {
	release, class := validateAndProject(buildVersion, checksumBytes, descriptorBytes)
	if class != descriptorValid || !releaseVersion.MatchString(buildVersion) || !hex64RE.MatchString(release.DescriptorSHA256) {
		return os.ErrInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensurePrivateRoot(); err != nil {
		return err
	}
	entries, err := c.ops.readDir(c.root)
	if err != nil {
		return err
	}
	pairs, _ := collectCachePairs(entries)
	for _, pair := range pairs {
		if pair.version == buildVersion && pair.digest != release.DescriptorSHA256 {
			return os.ErrExist
		}
	}
	if err := c.cleanupPartials(entries); err != nil {
		return err
	}
	if err := c.evictBeforeInsert(buildVersion, release.DescriptorSHA256); err != nil {
		return err
	}
	stem := buildVersion + "--" + release.DescriptorSHA256
	return c.writePair(stem, descriptorBytes, checksumBytes, now)
}

func (c *releaseCache) ensurePrivateRoot() error {
	info, err := c.ops.lstat(c.root)
	if errors.Is(err, os.ErrNotExist) {
		if err := c.ops.mkdirAll(c.root, cacheDirectoryMode); err != nil {
			return err
		}
		info, err = c.ops.lstat(c.root)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.ErrInvalid
	}
	return c.ops.chmod(c.root, cacheDirectoryMode)
}

func (c *releaseCache) cleanupPartials(entries []os.DirEntry) error {
	pairs, _ := collectCachePairs(entries)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			if err := removeIfPresent(c.ops, filepath.Join(c.root, entry.Name())); err != nil {
				return err
			}
		}
	}
	for _, pair := range pairs {
		if (pair.descriptorName == "") == (pair.checksumName == "") {
			continue
		}
		name := pair.descriptorName
		if name == "" {
			name = pair.checksumName
		}
		if err := removeIfPresent(c.ops, filepath.Join(c.root, name)); err != nil {
			return err
		}
	}
	return nil
}

type evictionCandidate struct {
	pair cachePair
	time time.Time
}

func (c *releaseCache) evictBeforeInsert(currentVersion, currentDigest string) error {
	entries, err := c.ops.readDir(c.root)
	if err != nil {
		return err
	}
	pairs, _ := collectCachePairs(entries)
	complete := make([]evictionCandidate, 0, len(pairs))
	currentExists := false
	for _, pair := range pairs {
		if pair.descriptorName == "" || pair.checksumName == "" {
			continue
		}
		if pair.version == currentVersion && pair.digest == currentDigest {
			currentExists = true
			continue
		}
		info, err := c.ops.lstat(filepath.Join(c.root, pair.checksumName))
		if err != nil {
			return err
		}
		complete = append(complete, evictionCandidate{pair: pair, time: info.ModTime()})
	}
	allowedOthers := cacheMaxPairs - 1
	if currentExists {
		allowedOthers = cacheMaxPairs - 1
	}
	sort.Slice(complete, func(i, j int) bool {
		if complete[i].time.Equal(complete[j].time) {
			return complete[i].pair.descriptorName < complete[j].pair.descriptorName
		}
		return complete[i].time.Before(complete[j].time)
	})
	for len(complete) > allowedOthers {
		candidate := complete[0]
		complete = complete[1:]
		if err := removeIfPresent(c.ops, filepath.Join(c.root, candidate.pair.checksumName)); err != nil {
			return err
		}
		if err := removeIfPresent(c.ops, filepath.Join(c.root, candidate.pair.descriptorName)); err != nil {
			return err
		}
	}
	return c.syncDirectory()
}

func (c *releaseCache) writePair(stem string, descriptorBytes, checksumBytes []byte, now time.Time) (returnErr error) {
	descriptorTemp, err := c.writeTemp(stem+".json", descriptorBytes)
	if err != nil {
		return err
	}
	defer func() { _ = removeIfPresent(c.ops, descriptorTemp) }()
	checksumTemp, err := c.writeTemp(stem+".sha256", checksumBytes)
	if err != nil {
		return err
	}
	defer func() { _ = removeIfPresent(c.ops, checksumTemp) }()

	descriptorPath := filepath.Join(c.root, stem+".json")
	checksumPath := filepath.Join(c.root, stem+".sha256")
	rollback := func() {
		_ = removeIfPresent(c.ops, checksumPath)
		_ = removeIfPresent(c.ops, descriptorPath)
		_ = c.syncDirectory()
	}
	if err := removeIfPresent(c.ops, checksumPath); err != nil {
		return err
	}
	if err := removeIfPresent(c.ops, descriptorPath); err != nil {
		return err
	}
	if err := c.ops.rename(descriptorTemp, descriptorPath); err != nil {
		rollback()
		return err
	}
	if err := c.ops.chtimes(descriptorPath, now, now); err != nil {
		rollback()
		return err
	}
	if err := c.syncDirectory(); err != nil {
		rollback()
		return err
	}
	if err := c.ops.rename(checksumTemp, checksumPath); err != nil {
		rollback()
		return err
	}
	if err := c.ops.chtimes(checksumPath, now, now); err != nil {
		rollback()
		return err
	}
	if err := c.syncDirectory(); err != nil {
		rollback()
		return err
	}
	return nil
}

func (c *releaseCache) writeTemp(label string, content []byte) (string, error) {
	file, err := c.ops.createTmp(c.root, ".tmp-"+label+"-")
	if err != nil {
		return "", err
	}
	name := file.Name()
	ok := false
	defer func() {
		if !ok {
			_ = file.Close()
			_ = removeIfPresent(c.ops, name)
		}
	}()
	if err := file.Chmod(cacheFileMode); err != nil {
		return "", err
	}
	written, err := file.Write(content)
	if err != nil {
		return "", err
	}
	if written != len(content) {
		return "", io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}

func (c *releaseCache) syncDirectory() error {
	directory, err := c.ops.open(c.root)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func removeIfPresent(ops fileOps, path string) error {
	err := ops.remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
