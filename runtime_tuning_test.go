package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanRuntimeTuning(t *testing.T) {
	tests := []struct {
		name string
		in   tuningInput
		want tuningPlan
	}{
		{"desktop 16 GiB", tuningInput{physicalBytes: 16 * gib}, tuningPlan{150, 4 * gib}},
		{"desktop 8 GiB", tuningInput{physicalBytes: 8 * gib}, tuningPlan{150, 2 * gib}},
		{"desktop 2 GiB floors at 1 GiB", tuningInput{physicalBytes: 2 * gib}, tuningPlan{150, 1 * gib}},
		{"desktop 128 GiB caps at 4 GiB", tuningInput{physicalBytes: 128 * gib}, tuningPlan{150, 4 * gib}},
		{"desktop unknown memory falls back to 2 GiB", tuningInput{}, tuningPlan{150, 2 * gib}},
		{"GOGC env wins", tuningInput{gogcSet: true, physicalBytes: 16 * gib}, tuningPlan{0, 4 * gib}},
		{"GOMEMLIMIT env wins", tuningInput{memLimitSet: true, physicalBytes: 16 * gib}, tuningPlan{150, 0}},
		{"both env set", tuningInput{gogcSet: true, memLimitSet: true}, tuningPlan{}},
		{"desktop in a container uses the cgroup limit", tuningInput{physicalBytes: 64 * gib, cgroupBytes: 1 * gib}, tuningPlan{150, int64(1*gib/100) * 85}},
		{"headless without limit changes nothing", tuningInput{headless: true, physicalBytes: 16 * gib}, tuningPlan{}},
		{"headless with cgroup limit", tuningInput{headless: true, cgroupBytes: 512 * mib}, tuningPlan{0, int64(512*mib/100) * 85}},
		{"headless never sets GC percent", tuningInput{headless: true, cgroupBytes: 2 * gib}, tuningPlan{0, int64(2*gib/100) * 85}},
		{"headless GOMEMLIMIT wins over cgroup", tuningInput{headless: true, memLimitSet: true, cgroupBytes: 2 * gib}, tuningPlan{}},
		{"tiny cgroup limit is ignored", tuningInput{headless: true, cgroupBytes: 16 * mib}, tuningPlan{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planRuntimeTuning(tt.in); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func writeCgroupFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadCgroupMemoryLimit(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  uint64
	}{
		{"v2 limit", map[string]string{"memory.max": "536870912\n"}, 536870912},
		{"v2 max means unlimited", map[string]string{"memory.max": "max\n"}, 0},
		{"v1 limit", map[string]string{"memory/memory.limit_in_bytes": "1073741824\n"}, 1073741824},
		{"v1 unlimited sentinel", map[string]string{"memory/memory.limit_in_bytes": "9223372036854771712\n"}, 0},
		{"v2 preferred over v1", map[string]string{"memory.max": "100000000", "memory/memory.limit_in_bytes": "200000000"}, 100000000},
		{"v2 unlimited falls through to v1", map[string]string{"memory.max": "max", "memory/memory.limit_in_bytes": "200000000"}, 200000000},
		{"garbage", map[string]string{"memory.max": "lots"}, 0},
		{"empty", map[string]string{"memory.max": ""}, 0},
		{"nothing", map[string]string{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tt.files {
				writeCgroupFile(t, root, rel, content)
			}
			if got := readCgroupMemoryLimit(root); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPhysicalMemoryIsPlausible(t *testing.T) {
	// Linux, macOS and Windows implement it; elsewhere it reports unknown (0).
	if v := physicalMemoryBytes(); v != 0 && v < 128*mib {
		t.Fatalf("implausible physical memory: %d", v)
	}
}
