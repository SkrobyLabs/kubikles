# Kubikles Project Context

Lightweight, high-performance desktop Kubernetes client. Go+React via Wails framework.

## Documentation Guidelines

**DO NOT** store in these docs:
- Line counts, file sizes, or any metrics that change with edits
- Counts of files (e.g., "38 files", "28 more files")
- Any statistics that become stale after code changes

**DO** store: file paths, purposes, patterns, and structural information.

## Tech Stack
- **Desktop**: Wails v2
- **Backend**: Go 1.24+, client-go
- **Frontend**: React 18, Vite, TailwindCSS
- **Editor**: Monaco | **Terminal**: xterm.js | **Graphs**: React Flow + dagre

## Quick File Reference

### Backend Core
| Purpose | File |
|---------|------|
| App struct & lifecycle | `app.go` |
| K8s API operations | `pkg/k8s/client.go` |
| Event batching | `eventcoalescer.go` |
| AI integration | `pkg/ai/` |
| MCP server | `pkg/mcp/server.go` |

### App Domain Files (split from app.go)
| Domain | File |
|--------|------|
| Watcher manager | `app_watchermgr.go` |
| Watch loops | `app_watchers.go` |
| Performance metrics | `app_perfmetrics.go` |
| Pods | `app_pods.go` |
| Deployments | `app_deployments.go` |
| StatefulSets | `app_statefulsets.go` |
| DaemonSets | `app_daemonsets.go` |
| ReplicaSets | `app_replicasets.go` |
| Jobs & CronJobs | `app_jobs.go` |
| Services | `app_services.go` |
| Ingresses | `app_ingresses.go` |
| ConfigMaps & Secrets | `app_configmaps.go` |
| Namespaces | `app_namespaces.go` |
| Nodes | `app_nodes.go` |
| Events | `app_events.go` |
| Storage (PVC/PV) | `app_storage.go` |
| CSI | `app_csi.go` |
| Custom Resources | `app_customresources.go` |
| RBAC | `app_rbac.go` |
| Network (HPA/PDB/NetPol) | `app_network.go` |
| Scheduling | `app_scheduling.go` |
| Webhooks | `app_webhooks.go` |
| Helm | `app_helm.go` |
| Port forwarding | `app_portforward.go` |
| Ingress forwarding | `app_ingressfwd.go` |
| Log streaming | `app_logs.go` |
| Terminal | `app_terminal.go` |
| File transfer | `app_filetransfer.go` |
| Prometheus | `app_prometheus.go` |
| Certificates | `app_certificates.go` |
| Diagnostics | `app_diagnostics.go` |
| AI assistant | `app_ai.go` |
| K8s context | `app_context.go` |
| Config settings | `app_config.go` |
| Themes | `app_themes.go` |
| Debug/logging | `app_debug.go` |
| Native dialogs | `app_dialogs.go` |

### K8s Package
| Purpose | File |
|---------|------|
| K8s API wrapper | `pkg/k8s/client.go` |
| Dependency graphs | `pkg/k8s/dependencies.go` |
| Resource diff | `pkg/k8s/diff.go` |
| Multi-resource logging | `pkg/k8s/multilog.go` |
| Flow timeline | `pkg/k8s/flowtimeline.go` |
| RBAC operations | `pkg/k8s/rbac.go` |
| File operations | `pkg/k8s/fileops.go` |

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
- **network**: services, ingresses, networkpolicies, endpoints, endpointslices, ingressclasses
- **config**: configmaps, secrets, hpas, pdbs, resourcequotas, leases, limitranges
- **storage**: pv, pvc, storageclass, csidrivers, csinodes
- **cluster**: nodes, namespaces, events, metrics, webhooks, priorityclasses
- **access-control**: roles, clusterroles, rolebindings, clusterrolebindings, serviceaccounts
- **customresources**: definitions, instances
- **helm**: releases, repos, oci
- **diagnostics**: resource comparison and diagnostics
- **portforwards**: port forward management UI

## Adding New K8s Resource
1. Backend: `pkg/k8s/client.go` (List/Get/Update/Delete) + `app_[domain].go` (expose + watcher)
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

## Required: Update Docs After Structural Changes

After adding/removing/renaming files in these locations, you MUST update the documentation:

| Changed Location | Update These Files |
|------------------|-------------------|
| `pkg/k8s/*.go` | All three docs below |
| `frontend/src/features/*/` | All three docs below |
| `frontend/src/hooks/*.js` | All three docs below |
| `frontend/src/context/*.jsx` | All three docs below |
| Root `*.go` files | All three docs below |

**Files to update:**
1. `docs/ai/README.md` - Project structure section
2. `.claude/rules/kubikles-context.md` - Quick reference tables
3. `.claude/skills/kubikles-ref/SKILL.md` - File locator tables

This is not optional. Do it in the same session as the structural change.
