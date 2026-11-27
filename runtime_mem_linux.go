//go:build linux

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// physicalMemoryBytes reads MemTotal from /proc/meminfo (reported in kB).
func physicalMemoryBytes() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			return kb * 1024
		}
	}
	return 0
}

func containerMemoryLimit() uint64 { return readCgroupMemoryLimit("/sys/fs/cgroup") }
