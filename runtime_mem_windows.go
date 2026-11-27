//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// physicalMemoryBytes calls GlobalMemoryStatusEx.
func physicalMemoryBytes() uint64 {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	var st memoryStatusEx
	st.Length = uint32(unsafe.Sizeof(st))
	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(&st)))
	if ret == 0 {
		return 0
	}
	return st.TotalPhys
}

func containerMemoryLimit() uint64 { return 0 }
