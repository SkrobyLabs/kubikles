package acceleratorrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveOutcomeMatrix(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	var corruptWire descriptor
	if err := json.Unmarshal(descriptorBytes, &corruptWire); err != nil {
		t.Fatal(err)
	}
	corruptWire.Compatibility.Mode = "range"
	corruptDescriptor, corruptChecksum := encodeTestPair(t, testBuildVersion, corruptWire)
	now := time.Unix(1_900_000_000, 0)
	prime := func(t *testing.T, root string) {
		t.Helper()
		if err := newReleaseCache(root, osFileOps).storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name         string
		buildVersion string
		steps        []responseStep
		prepare      func(*testing.T, string)
		ops          func(string) fileOps
		wantAvail    Availability
		wantSource   Source
		wantReason   UnavailableReason
		wantRequests int
	}{
		{name: "online valid", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: checksumBytes}, {status: 200, body: descriptorBytes}}, wantAvail: Available, wantSource: SourceNetwork, wantRequests: 2},
		{name: "online valid cache write failure", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: checksumBytes}, {status: 200, body: descriptorBytes}}, ops: func(string) fileOps {
			ops := osFileOps
			ops.mkdirAll = func(string, os.FileMode) error { return errors.New("cache write secret") }
			return ops
		}, wantAvail: Available, wantSource: SourceNetwork, wantRequests: 2},
		{name: "checksum 404 despite cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 404}}, prepare: prime, wantAvail: Unavailable, wantReason: DescriptorMissing, wantRequests: 1},
		{name: "checksum 410 despite cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 410}}, prepare: prime, wantAvail: Unavailable, wantReason: DescriptorMissing, wantRequests: 1},
		{name: "descriptor 404 despite cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: checksumBytes}, {status: 404}}, prepare: prime, wantAvail: Unavailable, wantReason: DescriptorMissing, wantRequests: 2},
		{name: "descriptor 410 despite cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: checksumBytes}, {status: 410}}, prepare: prime, wantAvail: Unavailable, wantReason: DescriptorMissing, wantRequests: 2},
		{name: "corrupt checksum despite cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: []byte("bad\n")}}, prepare: prime, wantAvail: Unavailable, wantReason: OnlineIntegrity, wantRequests: 1},
		{name: "corrupt descriptor despite cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: corruptChecksum}, {status: 200, body: corruptDescriptor}}, prepare: prime, wantAvail: Unavailable, wantReason: OnlineIntegrity, wantRequests: 2},
		{name: "oversized checksum despite cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: make([]byte, maxChecksumBytes+1)}}, prepare: prime, wantAvail: Unavailable, wantReason: OnlineIntegrity, wantRequests: 1},
		{name: "transient checksum with valid cache", buildVersion: testBuildVersion, steps: []responseStep{{err: context.DeadlineExceeded}}, prepare: prime, wantAvail: Available, wantSource: SourceCache, wantRequests: 1},
		{name: "transient descriptor with valid cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 200, body: checksumBytes}, {status: 503}}, prepare: prime, wantAvail: Available, wantSource: SourceCache, wantRequests: 2},
		{name: "transient missing cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 429}}, wantAvail: Unavailable, wantReason: NetworkUnavailable, wantRequests: 1},
		{name: "transient invalid cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 500}}, prepare: func(t *testing.T, root string) {
			_ = os.MkdirAll(root, cacheDirectoryMode)
			_ = os.WriteFile(filepath.Join(root, "unknown"), []byte("x"), cacheFileMode)
		}, wantAvail: Unavailable, wantReason: CacheInvalid, wantRequests: 1},
		{name: "transient expired cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 408}}, prepare: func(t *testing.T, root string) {
			writePairDirect(t, root, testBuildVersion, descriptorBytes, checksumBytes, now.Add(-cacheMaxAge-time.Second))
		}, wantAvail: Unavailable, wantReason: CacheInvalid, wantRequests: 1},
		{name: "transient conflicting cache", buildVersion: testBuildVersion, steps: []responseStep{{status: 425}}, prepare: func(t *testing.T, root string) {
			prime(t, root)
			var wire descriptor
			_ = json.Unmarshal(descriptorBytes, &wire)
			wire.Source.Commit = strings.Repeat("d", 40)
			wire.Schema = "https://raw.githubusercontent.com/SkrobyLabs/kubikles/" + wire.Source.Commit + "/release/accelerator-release.schema.json"
			otherDescriptor, otherChecksum := encodeTestPair(t, testBuildVersion, wire)
			writePairDirect(t, root, testBuildVersion, otherDescriptor, otherChecksum, now)
		}, wantAvail: Unavailable, wantReason: CacheInvalid, wantRequests: 1},
		{name: "transient cache IO", buildVersion: testBuildVersion, steps: []responseStep{{err: errors.New("network")}}, ops: func(string) fileOps {
			ops := osFileOps
			ops.lstat = func(string) (os.FileInfo, error) { return nil, errors.New("cache IO secret") }
			return ops
		}, wantAvail: Unavailable, wantReason: CacheIO, wantRequests: 1},
		{name: "invalid local build", buildVersion: "dev", wantAvail: Unavailable, wantReason: InvalidLocalBuild, wantRequests: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			if test.prepare != nil {
				test.prepare(t, root)
			}
			ops := osFileOps
			if test.ops != nil {
				ops = test.ops(root)
			}
			doer := &scriptedDoer{steps: append([]responseStep(nil), test.steps...)}
			resolver := newResolver(test.buildVersion, root, doer, ops, func() time.Time { return now })
			resolution := resolver.Resolve(context.Background())
			assertResolutionInvariant(t, resolution)
			wantSource := test.wantSource
			if test.wantAvail == Unavailable {
				wantSource = SourceNone
			}
			if resolution.Availability != test.wantAvail || resolution.Source != wantSource || resolution.Reason != test.wantReason || doer.count() != test.wantRequests {
				t.Fatalf("resolution = %#v, requests = %d", resolution, doer.count())
			}
		})
	}
}

func TestResolveOverrideAcceptsPrereleaseFromCustomDescriptorURL(t *testing.T) {
	const version = "v1.4.2-alpha.1"
	descriptorBytes, checksumBytes := pairForVersion(t, version)
	doer := &scriptedDoer{steps: []responseStep{{status: 200, body: checksumBytes}, {status: 200, body: descriptorBytes}}}
	resolver := newResolver(testBuildVersion, t.TempDir(), doer, osFileOps, time.Now)

	result := resolver.ResolveOverride(context.Background(), version, "https://artifacts.example.test/temporary.json")
	if result.Availability != Available || result.Source != SourceNetwork || result.Release.BuildVersion != version || doer.count() != 2 {
		t.Fatalf("resolution=%#v requests=%d", result, doer.count())
	}
	if got := doer.requests[0].URL.String(); got != "https://artifacts.example.test/temporary.json.sha256" {
		t.Fatalf("checksum URL=%q", got)
	}
	if got := doer.requests[1].URL.String(); got != "https://artifacts.example.test/temporary.json" {
		t.Fatalf("descriptor URL=%q", got)
	}
}

func TestResolveOfficialPrereleaseWithoutDevelopmentOverride(t *testing.T) {
	const version = "v1.4.2-alpha.1"
	descriptorBytes, checksumBytes := pairForVersion(t, version)
	doer := &scriptedDoer{steps: []responseStep{{status: 200, body: checksumBytes}, {status: 200, body: descriptorBytes}}}
	root := filepath.Join(t.TempDir(), "cache")
	resolver := newResolver(version, root, doer, osFileOps, time.Now)

	result := resolver.Resolve(context.Background())
	if result.Availability != Available || result.Source != SourceNetwork || result.Release.BuildVersion != version || doer.count() != 2 {
		t.Fatalf("resolution=%#v requests=%d", result, doer.count())
	}
	if _, state := newReleaseCache(root, osFileOps).loadExact(version, time.Now()); state != cacheOK {
		t.Fatalf("published prerelease was not cached: %v", state)
	}
}

func TestResolveBodyFailureFallsBackAndDescriptorChecksumBindsCache(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	now := time.Unix(1_900_000_000, 0)
	prime := func(t *testing.T, root string) {
		t.Helper()
		if err := newReleaseCache(root, osFileOps).storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, bodyErr := range []error{context.DeadlineExceeded, errors.New("connection reset"), io.ErrUnexpectedEOF} {
		t.Run(bodyErr.Error(), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			prime(t, root)
			body := &closeTrackingBody{reader: strings.NewReader("partial"), readErr: bodyErr}
			r := newResolver(testBuildVersion, root, doerFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil }), osFileOps, func() time.Time { return now })
			got := r.Resolve(context.Background())
			if got.Availability != Available || got.Source != SourceCache || !body.closed {
				t.Fatalf("body fallback = %#v closed=%v", got, body.closed)
			}
		})
	}
	t.Run("malformed checksum never falls back", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		prime(t, root)
		r := newResolver(testBuildVersion, root, &scriptedDoer{steps: []responseStep{{status: 200, body: []byte("malformed\n")}, {status: 503}}}, osFileOps, func() time.Time { return now })
		if got := r.Resolve(context.Background()); got.Reason != OnlineIntegrity {
			t.Fatalf("got %#v", got)
		}
	})
	t.Run("conflicting checksum cannot select cache", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		prime(t, root)
		other := append([]byte(nil), checksumBytes...)
		other[0] = '0'
		if other[0] == checksumBytes[0] {
			other[0] = '1'
		}
		r := newResolver(testBuildVersion, root, &scriptedDoer{steps: []responseStep{{status: 200, body: other}, {status: 503}}}, osFileOps, func() time.Time { return now })
		if got := r.Resolve(context.Background()); got.Reason != CacheInvalid {
			t.Fatalf("got %#v", got)
		}
	})
	t.Run("matching checksum falls back", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "cache")
		prime(t, root)
		r := newResolver(testBuildVersion, root, &scriptedDoer{steps: []responseStep{{status: 200, body: checksumBytes}, {status: 503}}}, osFileOps, func() time.Time { return now })
		if got := r.Resolve(context.Background()); got.Availability != Available || got.Source != SourceCache {
			t.Fatalf("got %#v", got)
		}
	})
}

func TestResolutionDiagnosticsAreRedacted(t *testing.T) {
	secret := "Bearer-UNIQUE-secret-token"
	descriptorBytes, checksumBytes := goldenPair(t)
	now := time.Unix(1_900_000_000, 0)
	scenarios := []Resolution{
		newResolver(testBuildVersion, filepath.Join(t.TempDir(), secret), doerFunc(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("transport %s", secret) }), osFileOps, func() time.Time { return now }).Resolve(context.Background()),
		newResolver(testBuildVersion, filepath.Join(t.TempDir(), "cache"), doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 500, Header: http.Header{"Authorization": []string{secret}}, Body: io.NopCloser(strings.NewReader(secret))}, nil
		}), osFileOps, func() time.Time { return now }).Resolve(context.Background()),
		newResolver(testBuildVersion, filepath.Join(t.TempDir(), "cache"), doerFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("%w: https://github.com/a?token=%s", errRedirectPolicy, secret)
		}), osFileOps, func() time.Time { return now }).Resolve(context.Background()),
	}
	writeFailOps := osFileOps
	writeFailOps.lstat = func(path string) (os.FileInfo, error) { return nil, fmt.Errorf("%s: %s", path, secret) }
	scenarios = append(scenarios, newResolver(testBuildVersion, filepath.Join(t.TempDir(), secret), doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }), writeFailOps, func() time.Time { return now }).Resolve(context.Background()))
	scenarios = append(scenarios, newResolver(testBuildVersion, filepath.Join(t.TempDir(), "cache"), &scriptedDoer{steps: []responseStep{{status: 200, body: checksumBytes}, {status: 200, body: descriptorBytes}}}, osFileOps, func() time.Time { return now }).Resolve(context.Background()))
	for _, resolution := range scenarios {
		assertResolutionInvariant(t, resolution)
		encoded, err := json.Marshal(resolution)
		if err != nil {
			t.Fatal(err)
		}
		formatted := fmt.Sprintf("%v %#v %s", resolution, resolution, encoded)
		if strings.Contains(formatted, secret) || strings.Contains(formatted, "token=") {
			t.Fatalf("secret escaped in %q", formatted)
		}
	}
}

func TestResolveOfflineCacheFallbackConcurrent(t *testing.T) {
	descriptorBytes, checksumBytes := goldenPair(t)
	root := filepath.Join(t.TempDir(), "cache")
	now := time.Unix(1_900_000_000, 0)
	cache := newReleaseCache(root, osFileOps)
	if err := cache.storeVerified(testBuildVersion, checksumBytes, descriptorBytes, now); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	resolver := newResolver(testBuildVersion, root, doerFunc(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, errors.New("offline") }), osFileOps, func() time.Time { return now })
	run := func(wantReason UnavailableReason) {
		var wg sync.WaitGroup
		results := make(chan Resolution, 64)
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- resolver.Resolve(context.Background()) }()
		}
		wg.Wait()
		close(results)
		var first *Resolution
		for resolution := range results {
			assertResolutionInvariant(t, resolution)
			if first == nil {
				copy := resolution
				first = &copy
			} else if resolution != *first {
				t.Errorf("non-deterministic results: %#v != %#v", resolution, *first)
			}
			if wantReason == "" && (resolution.Availability != Available || resolution.Source != SourceCache) {
				t.Errorf("offline fallback = %#v", resolution)
			}
			if wantReason != "" && resolution.Reason != wantReason {
				t.Errorf("corrupt fallback = %#v", resolution)
			}
		}
	}
	run("")
	descriptorPath, _ := cachePaths(t, root, testBuildVersion, descriptorBytes)
	corrupt := append([]byte(nil), descriptorBytes...)
	corrupt[len(corrupt)/2] ^= 1
	if err := os.WriteFile(descriptorPath, corrupt, cacheFileMode); err != nil {
		t.Fatal(err)
	}
	run(CacheInvalid)
	if calls.Load() != 128 {
		t.Fatalf("HTTP calls = %d", calls.Load())
	}
}
