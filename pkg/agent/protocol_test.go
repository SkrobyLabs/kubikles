package agent

import (
	"encoding/json"
	"testing"
)

func TestCheckBuildCompatibility(t *testing.T) {
	tests := []struct {
		name, desktop, accelerator string
		compatible                 bool
		status                     string
		action                     string
	}{
		{"exact match", "v1.3.0", "v1.3.0", true, "exact_match", "reuse_accelerator"},
		{"opaque exact match", "desktop@2026-08-01/opaque", "desktop@2026-08-01/opaque", true, "exact_match", "reuse_accelerator"},
		{"adjacent releases", "v1.3.0", "v1.2.2", false, "build_version_mismatch", "recreate_accelerator"},
		{"empty desktop", "", "v1.3.0", false, "missing_build_version", "recreate_accelerator"},
		{"empty accelerator", "v1.3.0", "", false, "missing_build_version", "recreate_accelerator"},
		{"both empty", "", "", false, "missing_build_version", "recreate_accelerator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckBuildCompatibility(tt.desktop, tt.accelerator)
			if got.Compatible != tt.compatible || string(got.Status) != tt.status || string(got.Action) != tt.action || got.DesktopBuildVersion != tt.desktop || got.AcceleratorBuildVersion != tt.accelerator {
				t.Fatalf("unexpected result: %+v", got)
			}
		})
	}
}

func TestCheckBuildCompatibilityIsLiteral(t *testing.T) {
	for _, versions := range [][2]string{{"v1.3.0", "V1.3.0"}, {"v1.3.0", " v1.3.0"}, {"v1.3.0", "v1.3.0 "}} {
		if result := CheckBuildCompatibility(versions[0], versions[1]); result.Compatible {
			t.Fatalf("expected literal mismatch for %#v: %+v", versions, result)
		}
	}
}

func TestAcceleratorInfoJSON(t *testing.T) {
	info := AcceleratorInfo{BuildIdentity: BuildIdentity{BuildVersion: "v1.3.0", Commit: "abcdef", Dirty: true}, Capabilities: []Capability{"secrets.list", "secrets.detail", "secrets.watch"}}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"buildIdentity":{"buildVersion":"v1.3.0","commit":"abcdef","dirty":true},"capabilities":["secrets.list","secrets.detail","secrets.watch"]}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}
