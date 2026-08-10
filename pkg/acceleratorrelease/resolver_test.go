package acceleratorrelease

import (
	"context"
	"testing"
)

func TestResolverBuildsVersionedDefaults(t *testing.T) {
	resolution := New("v1.4.0-alpha.2").Resolve(context.Background())
	if resolution.Availability != Available || resolution.Source != SourceBuiltIn {
		t.Fatalf("resolution = %#v", resolution)
	}
	if resolution.Release.ImageReference != "ghcr.io/skrobylabs/kubikles-accelerator:v1.4.0-alpha.2" || resolution.Release.ChartReference != "oci://ghcr.io/skrobylabs/helm/kubikles-accelerator:1.4.0-alpha.2" {
		t.Fatalf("release = %#v", resolution.Release)
	}
}

func TestResolverUsesCustomReferences(t *testing.T) {
	resolution := New("dev").ResolveOverride(context.Background(), "registry.example.test/team/accelerator:test", "registry.example.test/team/charts/accelerator:0.0.0-dev")
	if resolution.Availability != Available || resolution.Source != SourceCustom {
		t.Fatalf("resolution = %#v", resolution)
	}
	if resolution.Release.ImageReference != "registry.example.test/team/accelerator:test" || resolution.Release.ChartReference != "oci://registry.example.test/team/charts/accelerator:0.0.0-dev" {
		t.Fatalf("release = %#v", resolution.Release)
	}
}

func TestResolverRejectsInvalidReferences(t *testing.T) {
	resolver := New("v1.4.0")
	for _, refs := range [][2]string{{"https://example.test/image:tag", ""}, {"", "https://example.test/chart:tag"}, {"image", ""}, {"", "chart"}} {
		if resolution := resolver.ResolveOverride(context.Background(), refs[0], refs[1]); resolution.Availability != Unavailable || resolution.Reason != InvalidReference {
			t.Fatalf("accepted %q %q: %#v", refs[0], refs[1], resolution)
		}
	}
}
