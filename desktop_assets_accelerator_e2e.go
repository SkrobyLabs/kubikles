//go:build !headless && accelerator_e2e

package main

import "embed"

// Composed Accelerator acceptance exercises desktop lifecycle wiring without
// launching Wails or serving frontend assets.
var assets embed.FS
