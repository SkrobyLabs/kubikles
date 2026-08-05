package acceleratorrelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testBuildVersion = "v1.4.2"

func goldenPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	descriptorBytes, err := os.ReadFile(filepath.Join("testdata", "valid-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	checksumBytes, err := os.ReadFile(filepath.Join("testdata", "valid-v1.json.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	return descriptorBytes, checksumBytes
}

func pairForVersion(t *testing.T, version string) ([]byte, []byte) {
	t.Helper()
	descriptorBytes, _ := goldenPair(t)
	var wire descriptor
	if err := json.Unmarshal(descriptorBytes, &wire); err != nil {
		t.Fatal(err)
	}
	if !releaseVersion.MatchString(version) {
		t.Fatalf("invalid test version %q", version)
	}
	wire.BuildVersion = version
	wire.Source.GitTag = version
	wire.Compatibility.DesktopBuildVersion = version
	wire.Compatibility.AcceleratorBuildVersion = version
	wire.Chart.Version = strings.TrimPrefix(version, "v")
	wire.Chart.AppVersion = version
	return encodeTestPair(t, version, wire)
}

func encodeTestPair(t *testing.T, version string, wire descriptor) ([]byte, []byte) {
	t.Helper()
	descriptorBytes, err := json.MarshalIndent(wire, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	descriptorBytes = append(descriptorBytes, '\n')
	hash := sha256.Sum256(descriptorBytes)
	checksumBytes := []byte(hex.EncodeToString(hash[:]) + "  " + releaseAssetPrefix + version + ".json\n")
	return descriptorBytes, checksumBytes
}

func cachePaths(t *testing.T, root, version string, descriptorBytes []byte) (string, string) {
	t.Helper()
	hash := sha256.Sum256(descriptorBytes)
	stem := version + "--" + hex.EncodeToString(hash[:])
	return filepath.Join(root, stem+".json"), filepath.Join(root, stem+".sha256")
}

func writePairDirect(t *testing.T, root, version string, descriptorBytes, checksumBytes []byte, modTime time.Time) (string, string) {
	t.Helper()
	if err := os.MkdirAll(root, cacheDirectoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, cacheDirectoryMode); err != nil {
		t.Fatal(err)
	}
	descriptorPath, checksumPath := cachePaths(t, root, version, descriptorBytes)
	if err := os.WriteFile(descriptorPath, descriptorBytes, cacheFileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checksumPath, checksumBytes, cacheFileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(descriptorPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(checksumPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return descriptorPath, checksumPath
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

type responseStep struct {
	status int
	body   []byte
	err    error
}

type scriptedDoer struct {
	mu       sync.Mutex
	steps    []responseStep
	requests []*http.Request
}

func (d *scriptedDoer) Do(request *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.requests = append(d.requests, request.Clone(request.Context()))
	if len(d.steps) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	step := d.steps[0]
	d.steps = d.steps[1:]
	if step.err != nil {
		return nil, step.err
	}
	return &http.Response{StatusCode: step.status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(step.body)), Request: request}, nil
}

func (d *scriptedDoer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.requests)
}

func assertResolutionInvariant(t *testing.T, resolution Resolution) {
	t.Helper()
	zero := VerifiedRelease{}
	switch resolution.Availability {
	case Available:
		if resolution.Source != SourceNetwork && resolution.Source != SourceCache {
			t.Fatalf("available source = %q", resolution.Source)
		}
		if resolution.Reason != "" || resolution.Release == zero {
			t.Fatalf("invalid available resolution: %#v", resolution)
		}
		if !strings.Contains(resolution.Release.ImageReference, "@sha256:") || !strings.Contains(resolution.Release.ChartReference, "@sha256:") {
			t.Fatalf("mutable reference escaped: %#v", resolution.Release)
		}
	case Unavailable:
		if resolution.Source != SourceNone || resolution.Reason == "" || resolution.Release != zero {
			t.Fatalf("invalid unavailable resolution: %#v", resolution)
		}
	default:
		t.Fatalf("unknown availability: %#v", resolution)
	}
}
