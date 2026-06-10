# Kubikles Project Context

Lightweight, high-performance desktop Kubernetes client. Go+React via Wails framework.

## Documentation Guidelines

**DO NOT** store in any docs:
- Line counts, file sizes, or any metrics that change with edits
- Counts of files (e.g., "38 files", "28 more files")
- Any statistics that become stale after code changes

**DO** store: file paths, purposes, patterns, and structural information.

## Tech Stack
- **Desktop**: Wails v2
- **Backend**: Go 1.24+, client-go
- **Frontend**: React 18, Vite, TailwindCSS
- **Editor**: Monaco | **Terminal**: xterm.js | **Graphs**: React Flow + dagre

## File Index — Single Source of Truth

The complete project structure and file locator lives in **`docs/ai/README.md`**.
Do not duplicate the file tables here; read that doc (or run the `kubikles-ref`
skill, which loads it) to locate code.

## Adding New K8s Resource
1. Backend: `pkg/k8s/client.go` (List/Get/Update/Delete) + `app_[domain].go` (expose + watcher)
2. Generate: `wails generate module`
3. Hook: `frontend/src/hooks/use[Resource].tsx`
4. Feature: `frontend/src/features/{category}/{resource}/`
5. Register: `frontend/src/utils/resourceRegistry.ts`
6. Route: `App.tsx` + `Sidebar.tsx`

## Build Commands
```bash
make dev          # Development with hot-reload
make build        # Current platform (BUILD_TAGS=helm)
make build-lite   # Current platform without Helm
make build-all    # All platforms (parallel)
make test         # Frontend tests
make generate     # Regenerate dispatch_gen.go
make analyze-size # Binary size analysis
```

## Required: Update Docs After Structural Changes

After adding/removing/renaming files in `pkg/k8s/`, `frontend/src/features/`,
`frontend/src/hooks/`, `frontend/src/context/`, or root `*.go` files, update
**`docs/ai/README.md`** in the same session. That doc is the only file index;
this rule file and the `kubikles-ref` skill reference it rather than copying it.

This is not optional. Do it in the same session as the structural change.
