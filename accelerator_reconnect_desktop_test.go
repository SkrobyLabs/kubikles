//go:build !headless && helm && !accelerator

package main

import (
	"testing"
)

func TestDesktopAcceleratorReconnectorIsDormant(t *testing.T) {
	if newDesktopAcceleratorReconnector() == nil {
		t.Fatal("desktop reconnector constructor returned nil")
	}
}
