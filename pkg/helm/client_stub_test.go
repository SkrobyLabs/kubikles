//go:build !helm

package helm

import (
	"testing"
	"time"
)

func TestParseHelmTime(t *testing.T) {
	tests := []struct {
		input string
		want  time.Time
	}{
		{"2024-01-15 10:30:00.123456789 +0000 UTC", time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC)},
		{"2024-12-31 23:59:59.0 +0000 UTC", time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)},
		{"", time.Time{}},
		{"garbage", time.Time{}},
	}
	for _, tt := range tests {
		got := parseHelmTime(tt.input)
		if !got.Equal(tt.want) {
			t.Errorf("parseHelmTime(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSplitChartNameVersion(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantVersion string
	}{
		{"nginx-1.2.3", "nginx", "1.2.3"},
		{"my-app-chart-0.1.0", "my-app-chart", "0.1.0"},
		{"simple", "simple", ""},
		{"traefik-26.1.0", "traefik", "26.1.0"},
		{"kube-prometheus-stack-58.2.1", "kube-prometheus-stack", "58.2.1"},
		{"", "", ""},
	}
	for _, tt := range tests {
		name, version := splitChartNameVersion(tt.input)
		if name != tt.wantName || version != tt.wantVersion {
			t.Errorf("splitChartNameVersion(%q) = (%q, %q), want (%q, %q)",
				tt.input, name, version, tt.wantName, tt.wantVersion)
		}
	}
}

func TestCliReleaseToRelease(t *testing.T) {
	cli := cliRelease{
		Name:       "my-release",
		Namespace:  "default",
		Revision:   "3",
		Updated:    "2024-06-15 14:30:00.0 +0000 UTC",
		Status:     "deployed",
		Chart:      "nginx-1.2.3",
		AppVersion: "1.25.0",
	}

	r := cli.toRelease()

	if r.Name != "my-release" {
		t.Errorf("Name = %q, want %q", r.Name, "my-release")
	}
	if r.Revision != 3 {
		t.Errorf("Revision = %d, want 3", r.Revision)
	}
	if r.Chart != "nginx" {
		t.Errorf("Chart = %q, want %q", r.Chart, "nginx")
	}
	if r.ChartVersion != "1.2.3" {
		t.Errorf("ChartVersion = %q, want %q", r.ChartVersion, "1.2.3")
	}
	if r.Status != "deployed" {
		t.Errorf("Status = %q, want %q", r.Status, "deployed")
	}
}
