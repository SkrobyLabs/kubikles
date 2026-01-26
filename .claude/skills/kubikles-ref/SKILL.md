---
name: kubikles-ref
description: Get Kubikles codebase reference. Use when working on Kubikles to understand project structure, patterns, or locate files without scanning the entire codebase.
allowed-tools: Read
---

# Kubikles Quick Reference

Read the AI reference documentation for instant project context:

!`cat /Users/skroby/Documents/Source/projects/kubikles/docs/ai/README.md`

## Usage Notes

This reference covers:
- Project structure and key files
- Tech stack (Go + React + Wails)
- Key systems (event coalescing, watchers, port forwarding, terminals)
- Context providers and their APIs
- Data fetching and feature module patterns
- Step-by-step guide for adding new K8s resources
- Critical file mapping by task type
- Build commands and performance notes

For deeper exploration of specific areas, read the relevant source files directly:
- Backend core: `app.go`, `pkg/k8s/client.go`
- Frontend entry: `frontend/src/App.jsx`
- Context state: `frontend/src/context/*.jsx`
- Resource features: `frontend/src/features/*/`
