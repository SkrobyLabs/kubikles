//go:build !headless && !accelerator_e2e

package main

import "embed"

//go:embed all:frontend/dist
var assets embed.FS
