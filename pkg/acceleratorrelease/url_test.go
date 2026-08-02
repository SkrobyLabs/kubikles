package acceleratorrelease

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAssetURLsUseLiteralStableBuildVersion(t *testing.T) {
	descriptorURL, checksumURL, ok := assetURLs("v1.4.2")
	if !ok {
		t.Fatal("canonical version rejected")
	}
	want := "https://github.com/SkrobyLabs/kubikles/releases/download/v1.4.2/kubikles-accelerator-release-v1.4.2.json"
	if descriptorURL != want || checksumURL != want+".sha256" {
		t.Fatalf("URLs = %q, %q", descriptorURL, checksumURL)
	}
}

func TestAssetURLsRejectNonCanonicalBuildVersionsWithoutIO(t *testing.T) {
	invalid := []string{"", "dev", " v1.4.2", "v1.4.2 ", "V1.4.2", "v01.4.2", "v1.04.2", "v1.4.02", "1.4.2", "v1.4", "v1.4.2.0", "v1.4.2-rc.1", "v1.4.2+build", "latest", "main", "0123456789abcdef0123456789abcdef01234567", "v1.4.*", "v1.4.2/other", "v1.4.2?token=x", "v1.4.2\n"}
	for _, version := range invalid {
		t.Run(version, func(t *testing.T) {
			if descriptorURL, checksumURL, ok := assetURLs(version); ok || descriptorURL != "" || checksumURL != "" {
				t.Fatalf("accepted %q: %q %q", version, descriptorURL, checksumURL)
			}
			calls := 0
			resolver := newResolver(version, filepath.Join(t.TempDir(), "never-created"), doerFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, nil
			}), osFileOps, func() time.Time { return time.Unix(1, 0) })
			resolution := resolver.Resolve(context.Background())
			if resolution.Reason != InvalidLocalBuild || calls != 0 {
				t.Fatalf("resolution = %#v, HTTP calls = %d", resolution, calls)
			}
			if _, err := os.Stat(resolver.cacheRoot); !os.IsNotExist(err) {
				t.Fatalf("invalid build touched cache: %v", err)
			}
		})
	}
}
