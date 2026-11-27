//go:build !e2e && !headless

package main

import "context"

// The desktop test hook (e2e_hook.go) is compiled only with the "e2e" tag.

func e2eBindings() []interface{} { return nil }

func startE2EHook(context.Context) {}
