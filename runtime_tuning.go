package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
)

// Runtime tuning: GC percent and a soft memory limit, chosen per build and
// environment. GOGC and GOMEMLIMIT from the environment always win, and
// GOMAXPROCS is deliberately left alone (the runtime default already matches
// the CPU count and, on Go 1.25+, container CPU limits).

const (
	mib = 1 << 20
	gib = 1 << 30

	desktopGCPercent = 150

	// Desktop soft limit: a quarter of physical memory, within [1, 4] GiB.
	// A soft limit only makes the GC work harder near the ceiling; it never
	// fails allocations, so it just keeps a runaway list from ballooning RSS.
	desktopLimitFraction = 4
	desktopLimitMin      = 1 * gib
	desktopLimitMax      = 4 * gib
	desktopLimitFallback = 2 * gib

	// Server soft limit as a percentage of the container limit, leaving room
	// for stacks, mmap'd files and runtime overhead outside the Go heap.
	cgroupLimitPercent = 85
	cgroupLimitFloor   = 64 * mib
)

type tuningInput struct {
	headless      bool
	gogcSet       bool
	memLimitSet   bool
	physicalBytes uint64 // 0 when unknown
	cgroupBytes   uint64 // 0 when there is no container memory limit
}

type tuningPlan struct {
	gcPercent int   // 0 means leave unchanged
	memLimit  int64 // 0 means leave unchanged
}

// planRuntimeTuning decides what to apply. It is pure so every platform and
// environment combination can be tested.
func planRuntimeTuning(in tuningInput) tuningPlan {
	var plan tuningPlan

	// A longer GC interval trades memory for CPU, which suits an interactive
	// desktop app that holds large list results. It is not assumed for the
	// server build, which may share a memory-constrained pod.
	if !in.headless && !in.gogcSet {
		plan.gcPercent = desktopGCPercent
	}

	if in.memLimitSet {
		return plan
	}

	switch {
	case in.cgroupBytes >= cgroupLimitFloor:
		// Respect a container limit in every build; a desktop binary run with
		// -server inside a pod hits the same OOM killer.
		plan.memLimit = int64(in.cgroupBytes / 100 * cgroupLimitPercent)
	case in.headless:
		// No container limit and no desktop assumptions: use runtime defaults.
	case in.physicalBytes > 0:
		limit := in.physicalBytes / desktopLimitFraction
		if limit < desktopLimitMin {
			limit = desktopLimitMin
		}
		if limit > desktopLimitMax {
			limit = desktopLimitMax
		}
		plan.memLimit = int64(limit)
	default:
		plan.memLimit = desktopLimitFallback
	}
	return plan
}

// readCgroupMemoryLimit returns the container memory limit in bytes, or 0 when
// there is none or it cannot be read. root is the cgroup mount (normally
// /sys/fs/cgroup). cgroup v2 is tried first, then v1.
func readCgroupMemoryLimit(root string) uint64 {
	if v, ok := readLimitFile(filepath.Join(root, "memory.max")); ok {
		return v
	}
	if v, ok := readLimitFile(filepath.Join(root, "memory", "memory.limit_in_bytes")); ok {
		return v
	}
	return 0
}

// readLimitFile parses a cgroup limit file. "max" and the huge sentinel v1
// reports for "unlimited" yield ok=false.
func readLimitFile(path string) (uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(data))
	if text == "" || text == "max" {
		return 0, false
	}
	v, err := strconv.ParseUint(text, 10, 64)
	if err != nil || v == 0 {
		return 0, false
	}
	// cgroup v1 reports PAGE_COUNTER_MAX (~2^63) when unlimited.
	if v >= 1<<60 {
		return 0, false
	}
	return v, true
}

func init() {
	plan := planRuntimeTuning(tuningInput{
		headless:      isHeadlessBuild,
		gogcSet:       os.Getenv("GOGC") != "",
		memLimitSet:   os.Getenv("GOMEMLIMIT") != "",
		physicalBytes: physicalMemoryBytes(),
		cgroupBytes:   containerMemoryLimit(),
	})
	if plan.gcPercent > 0 {
		debug.SetGCPercent(plan.gcPercent)
	}
	if plan.memLimit > 0 {
		debug.SetMemoryLimit(plan.memLimit)
	}
	if plan.gcPercent > 0 || plan.memLimit > 0 {
		log.Printf("[Runtime] GC percent=%d, soft memory limit=%d MiB (0 = unchanged)", plan.gcPercent, plan.memLimit/mib)
	}
}
