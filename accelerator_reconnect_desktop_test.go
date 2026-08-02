//go:build !headless && helm && !accelerator

package main

import (
	"os"
	"strings"
	"testing"
)

func TestDesktopAcceleratorReconnectorIsDormant(t *testing.T) {
	if newDesktopAcceleratorReconnector() == nil {
		t.Fatal("desktop reconnector constructor returned nil")
	}
	source, err := os.ReadFile("accelerator_reconnect_desktop.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"NewApp", "App{", "router", "emit", "event", "Provision", "Uninstall", "Delete", "go ", "context."} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("dormant constructor acquired forbidden boundary %q", forbidden)
		}
	}
}
