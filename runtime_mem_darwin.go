//go:build darwin

package main

import (
	"encoding/binary"
	"syscall"
)

// physicalMemoryBytes reads hw.memsize. syscall.Sysctl returns the raw bytes
// of an integer value as a string with one trailing NUL trimmed, so the
// little-endian value is padded back to eight bytes before decoding.
func physicalMemoryBytes() uint64 {
	raw, err := syscall.Sysctl("hw.memsize")
	if err != nil || len(raw) == 0 || len(raw) > 8 {
		return 0
	}
	var buf [8]byte
	copy(buf[:], raw)
	return binary.LittleEndian.Uint64(buf[:])
}

func containerMemoryLimit() uint64 { return 0 }
