---
name: kubikles-locate
description: Locate Kubikles files by category. Use when you need to find specific files for a task like adding features, modifying backend, or changing UI components.
allowed-tools: Read, Glob
user-invocable: false
---

# Kubikles File Locator

Quick file location map for common development tasks:

## Backend (Go)

| Purpose | File |
|---------|------|
| All Wails bindings | `app.go` |
| K8s API operations | `pkg/k8s/client.go` |
| Dependency graphs | `pkg/k8s/dependencies.go` |
| Event batching | `eventcoalescer.go` |
| Log batching | `logcoalescer.go` |
| Port forwarding | `portforward.go` |
| Ingress forwarding | `ingressforward.go` |
| Metrics handling | `metricsrequests.go` |
| List request mgmt | `listrequests.go` |
| Theme system | `theme.go` |
| Helm operations | `pkg/helm/client.go` |
| Helm repositories | `pkg/helm/repo.go` |
| Terminal (Unix) | `pkg/terminal/session_unix.go` |
| Terminal (Windows) | `pkg/terminal/session_windows.go` |
| Terminal manager | `pkg/terminal/manager.go` |

## Frontend (React)

| Purpose | File |
|---------|------|
| App entry/routing | `frontend/src/App.jsx` |
| K8s state | `frontend/src/context/K8sContext.jsx` |
| UI state | `frontend/src/context/UIContext.jsx` |
| Config state | `frontend/src/context/ConfigContext.jsx` |
| Theme state | `frontend/src/context/ThemeContext.jsx` |
| Sidebar nav | `frontend/src/components/layout/Sidebar.jsx` |
| Bottom panel | `frontend/src/components/layout/BottomPanel.jsx` |
| Resource table | `frontend/src/components/shared/ResourceList.jsx` |
| YAML editor | `frontend/src/components/shared/YamlEditor.jsx` |
| Log viewer | `frontend/src/components/shared/log-viewer/` |
| Terminal | `frontend/src/components/shared/Terminal.jsx` |
| Dep graph | `frontend/src/components/shared/DependencyGraph.jsx` |
| Config editor | `frontend/src/components/shared/config-editor/` |
| Resource registry | `frontend/src/utils/resourceRegistry.js` |
| K8s helpers | `frontend/src/utils/k8s-helpers.js` |

## Feature Directories

| Resource Category | Location |
|-------------------|----------|
| Pods | `frontend/src/features/workloads/pods/` |
| Deployments | `frontend/src/features/workloads/deployments/` |
| StatefulSets | `frontend/src/features/workloads/statefulsets/` |
| DaemonSets | `frontend/src/features/workloads/daemonsets/` |
| Jobs | `frontend/src/features/workloads/jobs/` |
| CronJobs | `frontend/src/features/workloads/cronjobs/` |
| Services | `frontend/src/features/network/services/` |
| Ingresses | `frontend/src/features/network/ingresses/` |
| ConfigMaps | `frontend/src/features/config/configmaps/` |
| Secrets | `frontend/src/features/config/secrets/` |
| Nodes | `frontend/src/features/cluster/nodes/` |
| Namespaces | `frontend/src/features/cluster/namespaces/` |
| PVs | `frontend/src/features/storage/pv/` |
| PVCs | `frontend/src/features/storage/pvc/` |
| Helm | `frontend/src/features/helm/` |
| CRDs | `frontend/src/features/customresources/` |
| RBAC | `frontend/src/features/access-control/` |

## Hooks

| Hook | Purpose |
|------|---------|
| `useResource.js` | Generic resource fetching |
| `useResourceWatcher.js` | Subscribe to K8s watch events |
| `usePortForwards.js` | Port forward state |
| `useIngressForward.js` | Ingress forward state |
| `usePodMetrics.js` | Pod metrics |
| `useNodeMetrics.js` | Node metrics |
| `usePerformancePanel.jsx` | Performance monitoring |

For $ARGUMENTS, locate the relevant files from this map.
