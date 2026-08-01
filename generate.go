package main

//go:generate go run cmd/gen-dispatcher/main.go .
// Exported *App methods are dispatched unless immediately annotated //kubikles:dispatch exclude.
// A leading agent.AuthenticatedCallContext is injected by trusted dispatch and is never decoded from JSON.
