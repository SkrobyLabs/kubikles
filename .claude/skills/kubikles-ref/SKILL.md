---
name: kubikles-ref
description: Get Kubikles codebase reference. Use when working on Kubikles to understand project structure, patterns, or locate files without scanning the entire codebase.
allowed-tools: Read, Glob
user-invocable: true
---

# Kubikles Complete Reference

Read the AI reference documentation for full project context:

!`cat /Users/skroby/Documents/Source/projects/kubikles/docs/ai/README.md`

> **Maintenance Note**: If this skill or `docs/ai/README.md` becomes outdated due to codebase changes, update them to reflect the current structure.

---

# Quick File Locator

## Backend (Go)

### Core Application
| Purpose | File |
|---------|------|
| All Wails bindings | `app.go` |
| Entry point, menus | `main.go` |
| Desktop entry | `main_desktop.go` |
| Headless entry | `main_headless.go` |
| Server mode | `server_mode.go` |
| Event batching | `eventcoalescer.go` |
| Log batching | `logcoalescer.go` |
| Port forwarding | `portforward.go` |
| Ingress forwarding | `ingressforward.go` |
| Metrics handling | `metricsrequests.go` |
| List request mgmt | `listrequests.go` |
| Theme system | `theme.go` |

### Packages
| Purpose | Location |
|---------|----------|
| K8s API operations | `pkg/k8s/client.go` |
| Dependency graphs | `pkg/k8s/dependencies.go` |
| File operations | `pkg/k8s/fileops.go` |
| Helm operations | `pkg/helm/client.go` |
| Helm repositories | `pkg/helm/repo.go` |
| OCI registries | `pkg/helm/oci.go` |
| Terminal (Unix) | `pkg/terminal/session_unix.go` |
| Terminal (Windows) | `pkg/terminal/session_windows.go` |
| Terminal manager | `pkg/terminal/manager.go` |
| AI integration | `pkg/ai/` |
| Claude CLI | `pkg/ai/claude_cli.go` |
| AI manager | `pkg/ai/manager.go` |
| MCP server | `pkg/mcp/server.go` |
| Tool registry | `pkg/tools/registry.go` |
| Server API | `pkg/server/api.go` |

## Frontend (React)

### Context Providers
| Purpose | File |
|---------|------|
| K8s state | `frontend/src/context/K8sContext.jsx` |
| UI state | `frontend/src/context/UIContext.jsx` |
| Config state | `frontend/src/context/ConfigContext.jsx` |
| Theme state | `frontend/src/context/ThemeContext.jsx` |
| Menu state | `frontend/src/context/MenuContext.jsx` |
| Debug logging | `frontend/src/context/DebugContext.jsx` |
| Notifications | `frontend/src/context/NotificationContext.jsx` |
| AI Chat | `frontend/src/context/AIChatContext.jsx` |

### Shared Components
| Purpose | File |
|---------|------|
| Resource table | `frontend/src/components/shared/ResourceList.jsx` |
| YAML editor | `frontend/src/components/shared/YamlEditor.jsx` |
| Log viewer | `frontend/src/components/shared/log-viewer/` |
| Terminal | `frontend/src/components/shared/Terminal.jsx` |
| Dep graph | `frontend/src/components/shared/DependencyGraph.jsx` |
| Config editor | `frontend/src/components/shared/config-editor/` |
| Command palette | `frontend/src/components/shared/CommandPalette.jsx` |
| Bulk actions | `frontend/src/components/shared/BulkActionBar.jsx` |
| Performance panel | `frontend/src/components/shared/PerformancePanel.jsx` |

### Utils
| Purpose | File |
|---------|------|
| Resource registry | `frontend/src/utils/resourceRegistry.js` |
| K8s helpers | `frontend/src/utils/k8s-helpers.js` |
| Logger | `frontend/src/utils/Logger.js` |

## Feature Directories

| Category | Resources |
|----------|-----------|
| workloads | pods, deployments, statefulsets, daemonsets, replicasets, jobs, cronjobs |
| network | services, ingresses, networkpolicies, endpoints, endpointslices, ingressclasses |
| config | configmaps, secrets, hpas, pdbs, resourcequotas, leases, limitranges |
| storage | pv, pvc, storageclass, csidrivers, csinodes |
| cluster | nodes, namespaces, events, metrics, webhooks, priorityclasses |
| access-control | roles, clusterroles, rolebindings, clusterrolebindings, serviceaccounts |
| customresources | definitions, instances |
| helm | releases, repos, oci |

Path pattern: `frontend/src/features/{category}/{resource}/`

## Hooks
| Hook | Purpose |
|------|---------|
| `useResource.js` | Generic resource fetching |
| `useResourceWatcher.js` | Subscribe to K8s watch events |
| `useResourceEventHandler.js` | Handle resource events |
| `usePortForwards.js` | Port forward state |
| `useIngressForward.js` | Ingress forward state |
| `usePodMetrics.js` | Pod metrics |
| `useNodeMetrics.js` | Node metrics |
| `useClusterMetrics.js` | Cluster metrics |
| `useNamespaceMetrics.js` | Namespace metrics |
| `useCRDs.js` | CRD fetching |
| `useCustomResources.js` | Custom resource instances |
| `useHelmReleases.js` | Helm release state |
| `useBulkActions.js` | Bulk action handling |
| `useSelection.js` | Selection state |
| `useCommandPaletteItems.js` | Command palette items |
