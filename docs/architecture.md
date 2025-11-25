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

## Tech Stack

| Layer | Technology |
|-------|------------|
| Desktop Framework | Wails v2 |
| Backend | Go 1.24+ |
| Kubernetes Client | client-go |
| Frontend | React 18 + Vite |
| Styling | TailwindCSS |
| Code Editor | Monaco Editor |
| Terminal | xterm.js |
| Graphs | React Flow + dagre |

## Communication

### Wails Bindings

Go methods in `app.go` are automatically exposed to JavaScript:

```go
// Go (app.go)
func (a *App) ListPods(namespace string) ([]v1.Pod, error)
```

```javascript
// JavaScript
import { ListPods } from '../wailsjs/go/main/App';
const pods = await ListPods('default');
```

### IPC Events

For real-time updates, the backend emits events:

```go
// Go
runtime.EventsEmit(a.ctx, "pod-event", event)
```

```javascript
// JavaScript
window.runtime.EventsOn("pod-event", (event) => {
  // Handle event
});
```

## Backend Structure

```
pkg/
├── k8s/
│   ├── client.go        # Kubernetes operations
│   └── dependencies.go  # Dependency graph resolution
└── terminal/
    └── terminal.go      # WebSocket terminal service
```

The `Client` struct wraps the Kubernetes clientset and provides methods for:
- Listing resources (Pods, Deployments, Services, etc.)
- YAML operations (get/update)
- Resource deletion
- Log streaming
- Watcher setup for real-time updates

## Frontend Structure

```
frontend/src/
├── context/         # State management (K8sContext, UIContext)
├── features/        # Feature-based modules
│   ├── cluster/     # Nodes, Namespaces, Events
│   ├── workloads/   # Pods, Deployments, StatefulSets, etc.
│   ├── config/      # ConfigMaps, Secrets
│   ├── network/     # Services
│   └── storage/     # PV, PVC, StorageClass
├── components/
│   ├── layout/      # Sidebar, BottomPanel
│   └── shared/      # ResourceList, YamlEditor, LogViewer, etc.
├── hooks/           # Data fetching hooks
└── utils/           # Helpers, formatting, resource registry
```

### Key Components

- **ResourceList**: Universal data table with sorting, filtering, namespace selection
- **YamlEditor**: Monaco-based YAML editor for resources
- **LogViewer**: Real-time pod log streaming
- **Terminal**: WebSocket-based terminal for pod exec
- **DependencyGraph**: React Flow visualization of resource relationships

## Data Flow

1. **Initial Load**: React component calls Wails binding → Go fetches from K8s API → Returns to frontend
2. **Real-time Updates**: Go watcher detects changes → Emits IPC event → React updates state
3. **User Actions**: React calls Wails binding → Go executes K8s operation → Returns result

## Resource Patterns

Each Kubernetes resource type follows a consistent pattern:

- `use[Resource].js` - Data fetching hook with watcher
- `[Resource]List.jsx` - List view component
- `use[Resource]Actions.jsx` - Action handlers (edit, delete, etc.)

See [AI Reference](ai/README.md) for detailed patterns.
