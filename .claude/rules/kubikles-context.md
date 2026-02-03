# Kubikles Project Context

Lightweight, high-performance desktop Kubernetes client. Go+React via Wails framework.

## Tech Stack
- **Desktop**: Wails v2
- **Backend**: Go 1.24+, client-go
- **Frontend**: React 18, Vite, TailwindCSS
- **Editor**: Monaco | **Terminal**: xterm.js | **Graphs**: React Flow + dagre

## Quick File Reference

### Backend Core
| Purpose | File |
|---------|------|
| All Wails bindings | `app.go` |
| K8s API operations | `pkg/k8s/client.go` |
| Dependency graphs | `pkg/k8s/dependencies.go` |
| Event batching | `eventcoalescer.go` |
| Port forwarding | `portforward.go` |
| AI integration | `pkg/ai/` |
| MCP server | `pkg/mcp/server.go` |

### Frontend Core
| Purpose | File |
|---------|------|
| App entry/routing | `frontend/src/App.jsx` |
| K8s state | `frontend/src/context/K8sContext.jsx` |
| UI state | `frontend/src/context/UIContext.jsx` |
| Config state | `frontend/src/context/ConfigContext.jsx` |
| AI Chat | `frontend/src/context/AIChatContext.jsx` |
| Sidebar nav | `frontend/src/components/layout/Sidebar.jsx` |
| Resource table | `frontend/src/components/shared/ResourceList.jsx` |

### Feature Modules
Resources are at `frontend/src/features/{category}/{resource}/`:
- **workloads**: pods, deployments, statefulsets, daemonsets, replicasets, jobs, cronjobs
- **network**: services, ingresses, networkpolicies, endpoints, endpointslices
- **config**: configmaps, secrets, hpas, pdbs, resourcequotas, leases, limitranges
- **storage**: pv, pvc, storageclass, csidrivers, csinodes
- **cluster**: nodes, namespaces, events, metrics, webhooks, priorityclasses
- **access-control**: roles, clusterroles, rolebindings, clusterrolebindings, serviceaccounts
- **customresources**: definitions, instances
- **helm**: releases, repos, oci

## Adding New K8s Resource
1. Backend: `pkg/k8s/client.go` (List/Get/Update/Delete) + `app.go` (expose + watcher)
2. Generate: `wails generate module`
3. Hook: `frontend/src/hooks/use[Resource].js`
4. Feature: `frontend/src/features/{category}/{resource}/`
5. Register: `frontend/src/utils/resourceRegistry.js`
6. Route: `App.jsx` + `Sidebar.jsx`

## Build Commands
```bash
make dev          # Development with hot-reload
make build        # Current platform
make test         # Frontend tests
```

## Full Reference
For complete documentation: `docs/ai/README.md`

> **Maintenance**: Update this file and `docs/ai/README.md` when codebase structure changes.
