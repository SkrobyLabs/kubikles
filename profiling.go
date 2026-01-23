//go:build profiling

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
)

func init() {
	// Start pprof server on a separate port
	go func() {
		port := os.Getenv("PPROF_PORT")
		if port == "" {
			port = "6060"
		}
		fmt.Printf("Starting pprof server on http://localhost:%s/debug/pprof/\n", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			fmt.Printf("pprof server error: %v\n", err)
		}
	}()
}

// ProfilingCommands provides runtime profiling utilities accessible from frontend
type ProfilingCommands struct {
	cpuProfileFile *os.File // Stores the file handle for proper cleanup
}

// StartCPUProfile starts CPU profiling to a file
func (p *ProfilingCommands) StartCPUProfile(filename string) error {
	// Close any existing profile file first
	if p.cpuProfileFile != nil {
		pprof.StopCPUProfile()
		p.cpuProfileFile.Close()
		p.cpuProfileFile = nil
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return err
	}
	p.cpuProfileFile = f
	return nil
}

// StopCPUProfile stops CPU profiling and closes the file
func (p *ProfilingCommands) StopCPUProfile() {
	pprof.StopCPUProfile()
	if p.cpuProfileFile != nil {
		p.cpuProfileFile.Close()
		p.cpuProfileFile = nil
	}
}

// WriteHeapProfile writes a memory profile
func (p *ProfilingCommands) WriteHeapProfile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	runtime.GC() // Get up-to-date statistics
	return pprof.WriteHeapProfile(f)
}

// GetMemStats returns current memory statistics
func (p *ProfilingCommands) GetMemStats() map[string]uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return map[string]uint64{
		"Alloc":      m.Alloc,
		"TotalAlloc": m.TotalAlloc,
		"Sys":        m.Sys,
		"NumGC":      uint64(m.NumGC),
		"HeapAlloc":  m.HeapAlloc,
		"HeapSys":    m.HeapSys,
		"HeapIdle":   m.HeapIdle,
		"HeapInuse":  m.HeapInuse,
		"StackInuse": m.StackInuse,
		"StackSys":   m.StackSys,
	}
}

// GetGoroutineCount returns the number of goroutines
func (p *ProfilingCommands) GetGoroutineCount() int {
	return runtime.NumGoroutine()
}
