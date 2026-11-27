//go:build !headless

package main

// isHeadlessBuild selects server-appropriate runtime tuning.
const isHeadlessBuild = false
