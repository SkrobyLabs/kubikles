---
name: kubikles-ref
description: Get Kubikles codebase reference. Use when working on Kubikles to understand project structure, patterns, or locate files without scanning the entire codebase.
allowed-tools: Read, Glob
user-invocable: true
---

# Kubikles Complete Reference

Read the AI reference documentation for full project context:

!`cat /Users/skroby/Documents/Source/projects/kubikles/docs/ai/README.md`

> **Required**: When making structural changes, update this file along with `docs/ai/README.md` and `.claude/rules/kubikles-context.md` in the same session.
>
> **DO NOT** store line counts, file sizes, file counts, or any metrics that become stale after code changes.

---

# Quick File Locator

## Backend (Go)

### Core Application
| Purpose | File |
|---------|------|
| App struct & lifecycle | `app.go` |
| Entry point, menus | `main.go` |
| Desktop entry | `main_desktop.go` |
| Headless entry | `main_headless.go` |
| Server mode | `server_mode.go` |
| Event batching | `eventcoalescer.go` |
| Log batching | `logcoalescer.go` |

### App Domain Files (Wails bindings split by domain)
| Domain | File |
|--------|------|
| Watcher manager | `app_watchermgr.go` |
| Watch loops & events | `app_watchers.go` |
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
| K8s Events | `app_events.go` |
| Storage (PVC/PV) | `app_storage.go` |
| CSI drivers/nodes | `app_csi.go` |
| Custom Resources | `app_customresources.go` |
| RBAC | `app_rbac.go` |
| Network (HPA/PDB/NetPol) | `app_network.go` |
| Scheduling | `app_scheduling.go` |
| Webhooks | `app_webhooks.go` |
| Helm | `app_helm.go` |
| Port forwarding | `app_portforward.go` |
| Ingress forwarding | `app_ingressfwd.go` |
| Log streaming | `app_logs.go` |
| Terminal sessions | `app_terminal.go` |
| File transfer | `app_filetransfer.go` |
| Prometheus | `app_prometheus.go` |
| Certificates | `app_certificates.go` |
| Diagnostics | `app_diagnostics.go` |
| Issue detection | `app_issuedetector.go` |
| AI assistant | `app_ai.go` |
| K8s context switching | `app_context.go` |
| Config settings | `app_config.go` |
| Themes | `app_themes.go` |
| Debug/crash logging | `app_debug.go` |
| Native dialogs | `app_dialogs.go` |

### Supporting Types
| Purpose | File |
|---------|------|
| Port forward types | `portforward.go` |
| Ingress forward types | `ingressforward.go` |
| Metrics request mgmt | `metricsrequests.go` |
| List request mgmt | `listrequests.go` |
| Theme types | `theme.go` |

### Packages
| Purpose | Location |
|---------|----------|
| K8s API operations | `pkg/k8s/client.go` |
| Dependency graphs | `pkg/k8s/dependencies.go` |
| Resource diff/comparison | `pkg/k8s/diff.go` |
| File operations | `pkg/k8s/fileops.go` |
| Multi-resource logging | `pkg/k8s/multilog.go` |
| Flow timeline | `pkg/k8s/flowtimeline.go` |
| RBAC operations | `pkg/k8s/rbac.go` |
| Helm operations | `pkg/helm/client.go` |
| Helm repositories | `pkg/helm/repo.go` |
| OCI registries | `pkg/helm/oci.go` |
| Terminal (Unix) | `pkg/terminal/session_unix.go` |
| Terminal (Windows) | `pkg/terminal/session_windows.go` |
| Terminal manager | `pkg/terminal/manager.go` |
| AI integration | `pkg/ai/` |
| Claude CLI | `pkg/ai/claude_cli.go` |
| AI manager | `pkg/ai/manager.go` |
| Debug logging | `pkg/debug/logger.go` |
| Issue detector types | `pkg/issuedetector/types.go` |
| Issue detector rule interface | `pkg/issuedetector/rule.go` |
| Issue detector resource cache | `pkg/issuedetector/resourcecache.go` |
| Issue detector engine | `pkg/issuedetector/engine.go` |
| Issue detector YAML loader | `pkg/issuedetector/yamlloader.go` |
| Issue detector built-in rules | `pkg/issuedetector/rules_builtin.go` |
| Issue detector networking rules | `pkg/issuedetector/rules_networking.go` |
| Issue detector workload rules | `pkg/issuedetector/rules_workloads.go` |
| Issue detector storage rules | `pkg/issuedetector/rules_storage.go` |
| Issue detector security rules | `pkg/issuedetector/rules_security.go` |
| Issue detector config rules | `pkg/issuedetector/rules_config.go` |
| Issue detector deprecation rules | `pkg/issuedetector/rules_deprecation.go` |
| MCP server | `pkg/mcp/server.go` |
| Tool registry | `pkg/tools/registry.go` |
| Command execution tool | `pkg/tools/run_command.go` |
| Server API | `pkg/server/api.go` |

## Frontend (React)

### Context Providers
| Purpose | File |
|---------|------|
| K8s state | `frontend/src/context/K8sContext.tsx` |
| UI state | `frontend/src/context/UIContext.tsx` |
| Config state | `frontend/src/context/ConfigContext.tsx` |
| Theme state | `frontend/src/context/ThemeContext.tsx` |
| Menu state | `frontend/src/context/MenuContext.tsx` |
| Debug logging | `frontend/src/context/DebugContext.tsx` |
| Notifications | `frontend/src/context/NotificationContext.tsx` |
| AI Chat | `frontend/src/context/AIChatContext.tsx` |
| Issue Detection | `frontend/src/context/IssueDetectorContext.tsx` |

### Constants
| Purpose | File |
|---------|------|
| Menu structure | `frontend/src/constants/menuStructure.ts` |
| Sidebar layout utils | `frontend/src/constants/sidebarLayoutUtils.ts` |

### Shared Components
| Purpose | File |
|---------|------|
| Resource table | `frontend/src/components/shared/ResourceList.tsx` |
| YAML editor | `frontend/src/components/shared/YamlEditor.tsx` |
| Log viewer | `frontend/src/components/shared/log-viewer/` |
| Terminal | `frontend/src/components/shared/Terminal.tsx` |
| Dep graph | `frontend/src/components/shared/DependencyGraph.tsx` |
| Config editor | `frontend/src/components/shared/config-editor/` |
| Command palette | `frontend/src/components/shared/CommandPalette.tsx` |
| Bulk actions | `frontend/src/components/shared/BulkActionBar.tsx` |
| Performance panel | `frontend/src/components/shared/PerformancePanel.tsx` |

### Utils
| Purpose | File |
|---------|------|
| Resource registry | `frontend/src/utils/resourceRegistry.ts` |
| K8s helpers | `frontend/src/utils/k8s-helpers.ts` |
| Logger | `frontend/src/utils/Logger.ts` |
| K8s type definitions | `frontend/src/types/k8s.ts` |

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
| diagnostics | resource comparison, diagnostics, issue detection |
| portforwards | port forward management UI |

Path pattern: `frontend/src/features/{category}/{resource}/`

## Hooks
| Hook | Purpose |
|------|---------|
| `useResource.tsx` | Generic resource fetching (TypeScript generics) |
| `useResourceWatcher.tsx` | Subscribe to K8s watch events |
| `useResourceEventHandler.tsx` | Handle resource events |
| `usePortForwards.tsx` | Port forward state |
| `useIngressForward.tsx` | Ingress forward state |
| `usePodMetrics.tsx` | Pod metrics |
| `useNodeMetrics.tsx` | Node metrics |
| `useClusterMetrics.tsx` | Cluster metrics |
| `useNamespaceMetrics.tsx` | Namespace metrics |
| `useCRDs.tsx` | CRD fetching |
| `useCustomResources.tsx` | Custom resource instances |
| `useHelmReleases.tsx` | Helm release state |
| `useBulkActions.tsx` | Bulk action handling |
| `useSelection.tsx` | Selection state |
| `useCommandPaletteItems.tsx` | Command palette items |
| `useBaseResourceActions.tsx` | Base action handlers for all resources |
| `useIssueDetector.tsx` | Issue detection scan state and actions |
