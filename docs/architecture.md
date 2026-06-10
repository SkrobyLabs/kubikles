# Architecture

## Overview

Kubikles is a desktop application built with the [Wails](https://wails.io/) framework, combining a Go backend with a React frontend.

```
┌─────────────────────────────────────────────────────────┐
│                    Desktop Window                        │
│  ┌───────────────────────────────────────────────────┐  │
│  │                 React Frontend                     │  │
│  │  (Vite + TailwindCSS + Monaco Editor + xterm)     │  │
│  └───────────────────────────────────────────────────┘  │
│                          │                               │
│                    Wails Bindings                        │
│                    + IPC Events                          │
│                          │                               │
│  ┌───────────────────────────────────────────────────┐  │
│  │                  Go Backend                        │  │
│  │        (client-go + WebSocket Terminal)           │  │
│  └───────────────────────────────────────────────────┘  │
│                          │                               │
│                   Kubernetes API                         │
└─────────────────────────────────────────────────────────┘
```

> For the complete, authoritative file-by-file layout, see
> [`docs/ai/README.md`](ai/README.md). This document covers the high-level
> concepts; it deliberately does not duplicate the file index.

## Tech Stack

| Layer | Technology |
|-------|------------|
| Desktop Framework | Wails v2 |
| Backend | Go 1.24+ |
| Kubernetes Client | client-go |
| Frontend | React 18 + TypeScript + Vite |
| Styling | TailwindCSS |
| Code Editor | Monaco Editor |
| Terminal | xterm.js |
| Graphs | React Flow + dagre |

## Run Modes

The same `App` core runs in three modes (see `main_desktop.go`, `main_headless.go`, `server_mode.go`):

- **Desktop** — Wails window; events flow over the native IPC bridge.
- **Server** — HTTP/WebSocket server (`pkg/server/`) exposing the same methods for a remote/browser frontend.
- **Headless** — no UI; used for programmatic/MCP access.

Modes are unified behind an `events.Emitter` interface (`pkg/events/`), so backend code emits events without knowing whether it is talking to Wails IPC or a WebSocket.

## Communication

### Wails Bindings

Go methods on the `App` struct are exposed to JavaScript:

```go
// Go (app_pods.go)
func (a *App) ListPods(namespace string) ([]v1.Pod, error)
```

```javascript
// JavaScript
import { ListPods } from '../wailsjs/go/main/App';
const pods = await ListPods('default');
```

Method dispatch is code-generated into `dispatch_gen.go` (via `make generate`)
rather than using reflection, which keeps dead-code elimination effective and
the binary small. Regenerate it after adding or changing `App` methods.

### IPC Events

For real-time updates, the backend emits events through the active emitter:

```go
// Go
a.emitEvent("pod-event", event)
```

```javascript
// JavaScript
window.runtime.EventsOn("pod-event", (event) => {
  // Handle event
});
```

Watch events are batched in ~16ms windows (`eventcoalescer.go`, `logcoalescer.go`)
to cap IPC overhead at roughly 60fps; DELETE events are flushed immediately.

## Backend Structure

The `App` struct (`app.go`) is the composition root. Wails bindings are split by
domain into `app_*.go` files (e.g. `app_pods.go`, `app_logs.go`, `app_terminal.go`)
so no single file owns every method.

Domain logic lives under `pkg/`. The main packages:

| Package | Responsibility |
|---------|----------------|
| `pkg/k8s/` | client-go wrapper: list/get/update/delete, watch, logs, diff, dependency graphs, metrics, Prometheus |
| `pkg/terminal/` | PTY/conpty session lifecycle (`manager.go`, `session_unix.go`, `session_windows.go`) |
| `pkg/helm/` | Helm releases, repos, OCI (behind the `helm` build tag; stubs for lite builds) |
| `pkg/issuedetector/` | Rule-based cluster scanning engine (built-in Go rules + YAML custom rules) |
| `pkg/ai/`, `pkg/mcp/`, `pkg/tools/` | AI assistant, MCP server, and the allowlisted tool/command registry |
| `pkg/server/` | HTTP/WebSocket server for server mode |
| `pkg/events/` | Emitter abstraction shared across run modes |
| `pkg/compressedassets/` | gzip-aware static asset serving |
| `pkg/debug/`, `pkg/crashlog/` | Structured debug logging and crash capture |

The `Client` struct in `pkg/k8s/` wraps the clientset and provides resource
listing, YAML get/update, deletion, log streaming, and watcher setup.

### Build Tags

- `helm` (default, via `BUILD_TAGS`) compiles full Helm support; the lite build
  (`make build-lite`, `!helm`) swaps in stub implementations.

## Frontend Structure

```
frontend/src/
├── context/         # State management (K8sContext, UIContext, ConfigContext, …)
├── features/        # Feature-based modules, one folder per resource type
│   ├── workloads/   # pods, deployments, statefulsets, daemonsets, replicasets, jobs, cronjobs
│   ├── cluster/     # nodes, namespaces, events, metrics, webhooks, priorityclasses, topology
│   ├── config/      # configmaps, secrets, hpas, pdbs, resourcequotas, leases, limitranges
│   ├── network/     # services, ingresses, networkpolicies, endpoints, …
│   ├── storage/     # pv, pvc, storageclass, csidrivers, csinodes
│   ├── access-control/, customresources/, helm/, diagnostics/, portforwards/
├── components/
│   ├── layout/      # Sidebar, BottomPanel
│   └── shared/      # ResourceList, YamlEditor, LogViewer, Terminal, DependencyGraph, …
├── hooks/           # Data fetching hooks (TypeScript)
└── utils/           # Helpers, formatting, resource registry
```

### Key Components

- **ResourceList**: Universal data table with sorting, filtering, virtualization, column config, and saved views
- **YamlEditor**: Monaco-based YAML editor for resources
- **LogViewer**: Real-time pod log streaming
- **Terminal**: WebSocket-based terminal for pod exec
- **DependencyGraph**: React Flow visualization of resource relationships

## Data Flow

1. **Initial Load**: React component calls a Wails binding → Go fetches from the K8s API → returns to the frontend
2. **Real-time Updates**: Go watcher detects a change → emits a (coalesced) IPC event → React updates state
3. **User Actions**: React calls a Wails binding → Go executes the K8s operation → returns the result

## Resource Patterns

Each Kubernetes resource type follows a consistent frontend pattern under
`features/[category]/[resource]/`:

- `use[Resource].tsx` — data-fetching hook with a reference-counted watcher
- `[Resource]List.tsx` — list view built on `ResourceList`
- `use[Resource]Actions.tsx` — action handlers (edit, delete, view YAML, …)

Watchers start on the first subscriber and clean up shortly after the last one
unsubscribes. See [AI Reference](ai/README.md) for the full patterns and the
step-by-step guide to adding a new resource.
