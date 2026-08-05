// Package acceleratorrelease resolves the immutable Accelerator release for this desktop build.
package acceleratorrelease

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Availability string

const (
	Available   Availability = "available"
	Unavailable Availability = "unavailable"
)

type Source string

const (
	SourceNone    Source = "none"
	SourceNetwork Source = "network"
	SourceCache   Source = "cache"
)

type UnavailableReason string

const (
	InvalidLocalBuild  UnavailableReason = "invalid_local_build"
	DescriptorMissing  UnavailableReason = "descriptor_missing"
	NetworkUnavailable UnavailableReason = "network_unavailable"
	OnlineIntegrity    UnavailableReason = "online_integrity"
	CacheInvalid       UnavailableReason = "cache_invalid"
	CacheIO            UnavailableReason = "cache_io"
)

type VerifiedRelease struct {
	BuildVersion     string `json:"buildVersion"`
	SourceCommit     string `json:"sourceCommit"`
	DescriptorSHA256 string `json:"descriptorSHA256"`
	ImageReference   string `json:"imageReference"`
	ChartReference   string `json:"chartReference"`
}

type Resolution struct {
	Availability Availability      `json:"availability"`
	Source       Source            `json:"source"`
	Reason       UnavailableReason `json:"reason,omitempty"`
	Release      VerifiedRelease   `json:"release,omitempty"`
}

var stableVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var releaseVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Resolver struct {
	buildVersion string
	cacheRoot    string
	client       httpDoer
	cache        *releaseCache
	now          func() time.Time
}

func New(buildVersion, cacheRoot string) *Resolver {
	return newResolver(buildVersion, cacheRoot, newHTTPClient(), osFileOps, time.Now)
}

func newResolver(buildVersion, cacheRoot string, client httpDoer, ops fileOps, now func() time.Time) *Resolver {
	return &Resolver{
		buildVersion: buildVersion,
		cacheRoot:    cacheRoot,
		client:       client,
		cache:        newReleaseCache(cacheRoot, ops),
		now:          now,
	}
}

func unavailable(reason UnavailableReason) Resolution {
	return Resolution{Availability: Unavailable, Source: SourceNone, Reason: reason}
}

func available(source Source, release VerifiedRelease) Resolution {
	return Resolution{Availability: Available, Source: source, Release: release}
}

func (r *Resolver) Resolve(ctx context.Context) Resolution {
	if !stableVersion.MatchString(r.buildVersion) {
		return unavailable(InvalidLocalBuild)
	}
	descriptorURL, checksumURL, ok := assetURLs(r.buildVersion)
	if !ok {
		return unavailable(InvalidLocalBuild)
	}
	checksumBytes, checksumClass := fetchAsset(ctx, r.client, checksumURL, maxChecksumBytes)
	switch checksumClass {
	case fetchMissing:
		return unavailable(DescriptorMissing)
	case fetchIntegrity:
		return unavailable(OnlineIntegrity)
	case fetchTransient:
		return r.resolveFromCache()
	}
	checksumDigest, checksumOK := parseChecksumLine(checksumBytes, releaseAssetPrefix+r.buildVersion+".json")
	if !checksumOK {
		return unavailable(OnlineIntegrity)
	}
	descriptorBytes, descriptorFetchClass := fetchAsset(ctx, r.client, descriptorURL, maxDescriptorBytes)
	switch descriptorFetchClass {
	case fetchMissing:
		return unavailable(DescriptorMissing)
	case fetchIntegrity:
		return unavailable(OnlineIntegrity)
	case fetchTransient:
		return r.resolveFromCacheDigest(checksumDigest)
	}
	release, descriptorClass := validateAndProject(r.buildVersion, checksumBytes, descriptorBytes)
	if descriptorClass != descriptorValid {
		return unavailable(OnlineIntegrity)
	}
	_ = r.cache.storeVerified(r.buildVersion, checksumBytes, descriptorBytes, r.now())
	return available(SourceNetwork, release)
}

// ResolveOverride resolves an explicitly selected development release. The
// normal resolver remains stable-only; this path requires a canonical version
// and an optional explicit HTTPS descriptor URL and deliberately bypasses the
// production cache.
func (r *Resolver) ResolveOverride(ctx context.Context, buildVersion, descriptorURL string) Resolution {
	if r == nil || !releaseVersion.MatchString(buildVersion) {
		return unavailable(InvalidLocalBuild)
	}
	if descriptorURL == "" && stableVersion.MatchString(buildVersion) {
		clone := *r
		clone.buildVersion = buildVersion
		return clone.Resolve(ctx)
	}
	if descriptorURL == "" {
		name := releaseAssetPrefix + buildVersion + ".json"
		descriptorURL = "https://github.com/SkrobyLabs/kubikles/releases/download/" + buildVersion + "/" + name
	}
	u, err := url.Parse(descriptorURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || strings.TrimSpace(descriptorURL) != descriptorURL {
		return unavailable(InvalidLocalBuild)
	}
	checksumLocation := *u
	checksumLocation.Path += ".sha256"
	checksumBytes, checksumClass := fetchAsset(ctx, r.client, checksumLocation.String(), maxChecksumBytes)
	if checksumClass == fetchMissing {
		return unavailable(DescriptorMissing)
	}
	if checksumClass != fetchOK {
		if checksumClass == fetchIntegrity {
			return unavailable(OnlineIntegrity)
		}
		return unavailable(NetworkUnavailable)
	}
	name := releaseAssetPrefix + buildVersion + ".json"
	if _, ok := parseChecksumLine(checksumBytes, name); !ok {
		return unavailable(OnlineIntegrity)
	}
	descriptorBytes, descriptorClass := fetchAsset(ctx, r.client, descriptorURL, maxDescriptorBytes)
	if descriptorClass == fetchMissing {
		return unavailable(DescriptorMissing)
	}
	if descriptorClass != fetchOK {
		if descriptorClass == fetchIntegrity {
			return unavailable(OnlineIntegrity)
		}
		return unavailable(NetworkUnavailable)
	}
	release, class := validateAndProject(buildVersion, checksumBytes, descriptorBytes)
	if class != descriptorValid {
		return unavailable(OnlineIntegrity)
	}
	return available(SourceNetwork, release)
}

func (r *Resolver) resolveFromCache() Resolution {
	return r.resolveFromCacheDigest("")
}

func (r *Resolver) resolveFromCacheDigest(digest string) Resolution {
	release, state := r.cache.loadExactDigest(r.buildVersion, digest, r.now())
	switch state {
	case cacheOK:
		return available(SourceCache, release)
	case cacheBad:
		return unavailable(CacheInvalid)
	case cacheIO:
		return unavailable(CacheIO)
	default:
		return unavailable(NetworkUnavailable)
	}
}
