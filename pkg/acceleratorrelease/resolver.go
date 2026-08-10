// Package acceleratorrelease selects the image and chart used by Accelerator.
package acceleratorrelease

import (
	"context"
	"regexp"
	"strings"
)

const (
	defaultImageRepository = "ghcr.io/skrobylabs/kubikles-accelerator"
	defaultChartRepository = "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator"
)

type Availability string

const (
	Available   Availability = "available"
	Unavailable Availability = "unavailable"
)

type Source string

const (
	SourceNone    Source = "none"
	SourceBuiltIn Source = "built_in"
	SourceCustom  Source = "custom"
)

type UnavailableReason string

const (
	InvalidLocalBuild UnavailableReason = "invalid_local_build"
	InvalidReference  UnavailableReason = "invalid_reference"
)

type VerifiedRelease struct {
	BuildVersion   string `json:"buildVersion"`
	ImageReference string `json:"imageReference"`
	ChartReference string `json:"chartReference"`
}

type Resolution struct {
	Availability Availability      `json:"availability"`
	Source       Source            `json:"source"`
	Reason       UnavailableReason `json:"reason,omitempty"`
	Release      VerifiedRelease   `json:"release,omitempty"`
}

type Resolver struct{ buildVersion string }

var registryReference = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+(?::[A-Za-z0-9_][A-Za-z0-9._-]{0,127}|@sha256:[0-9a-f]{64})$`)

func New(buildVersion string) *Resolver {
	return &Resolver{buildVersion: strings.TrimSpace(buildVersion)}
}

func DefaultReferences(buildVersion string) (imageReference, chartReference string, ok bool) {
	version := strings.TrimSpace(buildVersion)
	if version == "" || len(version) > 128 || strings.ContainsAny(version, " /:@") {
		return "", "", false
	}
	imageReference = defaultImageRepository + ":" + version
	chartVersion := strings.TrimPrefix(version, "v")
	chartReference = defaultChartRepository + ":" + chartVersion
	return imageReference, chartReference, true
}

func NormalizeImageReference(value string) (string, bool) {
	value = strings.TrimSpace(value)
	return value, value != "" && len(value) <= 512 && registryReference.MatchString(value)
}

func NormalizeChartReference(value string) (string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "oci://")
	if value == "" || len(value) > 512 || !registryReference.MatchString(value) {
		return "", false
	}
	return "oci://" + value, true
}

func (r *Resolver) Resolve(ctx context.Context) Resolution {
	if ctx == nil || ctx.Err() != nil || r == nil {
		return Resolution{Availability: Unavailable, Source: SourceNone, Reason: InvalidLocalBuild}
	}
	imageReference, chartReference, ok := DefaultReferences(r.buildVersion)
	if !ok {
		return Resolution{Availability: Unavailable, Source: SourceNone, Reason: InvalidLocalBuild}
	}
	return Resolution{Availability: Available, Source: SourceBuiltIn, Release: VerifiedRelease{BuildVersion: r.buildVersion, ImageReference: imageReference, ChartReference: chartReference}}
}

func (r *Resolver) ResolveOverride(ctx context.Context, imageReference, chartReference string) Resolution {
	resolution := r.Resolve(ctx)
	if resolution.Availability != Available {
		return resolution
	}
	if imageReference != "" {
		var ok bool
		resolution.Release.ImageReference, ok = NormalizeImageReference(imageReference)
		if !ok {
			return Resolution{Availability: Unavailable, Source: SourceNone, Reason: InvalidReference}
		}
		resolution.Source = SourceCustom
	}
	if chartReference != "" {
		var ok bool
		resolution.Release.ChartReference, ok = NormalizeChartReference(chartReference)
		if !ok {
			return Resolution{Availability: Unavailable, Source: SourceNone, Reason: InvalidReference}
		}
		resolution.Source = SourceCustom
	}
	return resolution
}
