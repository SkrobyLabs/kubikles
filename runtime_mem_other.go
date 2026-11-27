//go:build !darwin && !linux && !windows

package main

func physicalMemoryBytes() uint64  { return 0 }
func containerMemoryLimit() uint64 { return 0 }
