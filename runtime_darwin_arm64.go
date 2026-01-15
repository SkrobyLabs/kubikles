//go:build darwin && arm64

package main

import (
	"os"
	"runtime"
	"runtime/debug"
)

func init() {
	// Apple Silicon Runtime Optimizations
	//
	// Apple Silicon has unified memory architecture where CPU and GPU
	// share the same memory pool. This allows for more aggressive GC
	// settings since memory pressure is handled differently.

	// Only apply if not already set by user
	if os.Getenv("GOGC") == "" {
		// Increase GC target percentage for Apple Silicon
		// Default is 100, meaning GC triggers when heap doubles
		// 150 means GC triggers at 2.5x live heap, reducing GC frequency
		// This is safe on Apple Silicon due to unified memory and fast GC
		debug.SetGCPercent(150)
	}

	// Set memory limit if not already set (Go 1.19+)
	// On Apple Silicon, we can be more generous with memory
	if os.Getenv("GOMEMLIMIT") == "" {
		// Allow up to 2GB before soft limit kicks in
		// This prevents unnecessary GC pressure on systems with plenty of RAM
		debug.SetMemoryLimit(2 * 1024 * 1024 * 1024) // 2GB
	}

	// Ensure GOMAXPROCS is set appropriately
	// Apple Silicon efficiency cores are still very capable
	// runtime.GOMAXPROCS is usually set correctly, but we ensure it
	if os.Getenv("GOMAXPROCS") == "" {
		// Use all available cores (performance + efficiency)
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
}
