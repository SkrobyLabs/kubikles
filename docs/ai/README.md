# Kubikles AI Reference

Quick-access reference for AI assistants. This document eliminates the need to re-scan the codebase.

> **Required**: When adding/removing files in `pkg/k8s/`, `frontend/src/features/`, `frontend/src/hooks/`, `frontend/src/context/`, or root `.go` files, update this document and `.claude/rules/kubikles-context.md` and `.claude/skills/kubikles-ref/SKILL.md` in the same session.

> **DO NOT** store line counts, file sizes, file counts, or any metrics that become stale after code changes.

## Identity

**Kubikles** - Lightweight, high-performance desktop Kubernetes client. Go+React via Wails framework. Alternative to Lens.

## Tech Stack

| Layer | Tech |
|-------|------|
| Desktop | Wails v2 |
| Backend | Go 1.24+, client-go |
| Frontend | React 18, Vite, TailwindCSS |
| Editor | Monaco |
| Terminal | xterm.js (WebGL) |
| Graphs | React Flow + dagre |

## Project Structure

```
kubikles/
├── main.go                 # Entry point, Wails setup, menus
├── main_desktop.go         # Desktop mode entry
├── main_headless.go        # Headless mode entry
├── server_mode.go          # Server mode logic
├── app.go                  # App struct & lifecycle
├── app_*.go                # Domain-specific Wails bindings
│   ├── app_watchermgr.go   # ResourceWatcherManager
│   ├── app_watchers.go     # Watch loops, event types
│   ├── app_perfmetrics.go  # Performance metrics
│   ├── app_pods.go         # Pod operations
│   ├── app_deployments.go  # Deployment operations
│   ├── app_services.go     # Service operations
│   ├── app_helm.go         # Helm releases/repos
│   ├── app_logs.go         # Log streaming
│   ├── app_terminal.go     # Terminal sessions
│   ├── app_ai.go           # AI assistant
│   └── ...                 # (see rule file for complete list)
├── runtime_darwin_arm64.go # Apple Silicon runtime tuning
├── runtime_other.go        # Other platform runtime
├── eventcoalescer.go       # 16ms event batching for IPC efficiency
├── logcoalescer.go         # Log streaming batching
├── portforward.go          # Port forward manager types
├── ingressforward.go       # Ingress forwarding types
├── metricsrequests.go      # Prometheus metrics handling
├── listrequests.go         # Cancellable K8s list requests
├── theme.go                # Theme manager types
├── profiling.go            # PGO profiling support
├── version.go              # Version info
│
├── pkg/
│   ├── k8s/
│   │   ├── client.go       # K8s API wrapper - all resource operations
│   │   ├── dependencies.go # Dependency graph computation
│   │   ├── diff.go         # Resource diff/comparison
│   │   ├── fileops.go      # File operations for K8s resources
│   │   ├── flowtimeline.go # Flow timeline for resource events
│   │   ├── multilog.go     # Multi-resource log streaming
│   │   └── rbac.go         # RBAC operations
│   ├── terminal/
│   │   ├── manager.go      # Session lifecycle
│   │   ├── session_unix.go # Unix/macOS PTY
│   │   └── session_windows.go # Windows conpty
│   ├── helm/
│   │   ├── client.go       # Helm operations
│   │   ├── oci.go          # OCI registry
│   │   └── repo.go         # Repository management
│   ├── ai/                 # AI integration
│   │   ├── claude_cli.go   # Claude CLI integration
│   │   ├── manager.go      # AI session manager
│   │   └── provider.go     # Provider abstraction
│   ├── mcp/                # MCP server
│   │   └── server.go       # MCP protocol implementation
│   ├── tools/              # Tool registry for AI
│   │   ├── registry.go     # Tool registration
│   │   └── tools.go        # Tool implementations
│   ├── server/             # Server mode
│   │   ├── api.go          # REST API handlers
│   │   └── server.go       # HTTP server
│   ├── events/             # Event system
│   │   └── emitter.go      # Event emitter
│   ├── hosts/              # Platform-specific hosts file
│   ├── certviewer/         # Certificate inspection
│   └── crashlog/           # Crash logging
│
├── frontend/src/
│   ├── App.jsx             # Root component, providers, view routing
│   ├── main.jsx            # Entry, Monaco config
│   ├── context/
│   │   ├── K8sContext.jsx      # K8s state (contexts, namespaces, CRDs)
│   │   ├── UIContext.jsx       # UI state (tabs, modals, panels)
│   │   ├── ConfigContext.jsx   # User settings, port forwards
│   │   ├── ThemeContext.jsx    # Active theme
│   │   ├── MenuContext.jsx     # Context menus
│   │   ├── DebugContext.jsx    # Debug logging
│   │   ├── NotificationContext.jsx # Toast notifications
│   │   └── AIChatContext.jsx   # AI chat integration
│   ├── features/
│   │   ├── workloads/      # pods/, deployments/, statefulsets/, daemonsets/,
│   │   │                   # replicasets/, jobs/, cronjobs/
│   │   ├── cluster/        # nodes/, namespaces/, events/, metrics/, webhooks/,
│   │   │                   # priorityclasses/
│   │   ├── config/         # configmaps/, secrets/, hpas/, pdbs/, resourcequotas/,
│   │   │                   # leases/, limitranges/
│   │   ├── storage/        # pv/, pvc/, storageclass/, csidrivers/, csinodes/
│   │   ├── network/        # services/, ingresses/, networkpolicies/, endpoints/,
│   │   │                   # endpointslices/, ingressclasses/
│   │   ├── access-control/ # roles/, clusterroles/, rolebindings/,
│   │   │                   # clusterrolebindings/, serviceaccounts/
│   │   ├── customresources/ # CRDs and custom instances
│   │   ├── helm/           # Helm releases, repos, OCI
│   │   ├── diagnostics/    # Resource comparison, diagnostics
│   │   └── portforwards/   # Port forward management UI
│   ├── components/
│   │   ├── layout/         # Sidebar.jsx, BottomPanel.jsx
│   │   └── shared/         # ResourceList.jsx, YamlEditor.jsx, LogViewer.jsx,
│   │                       # Terminal.jsx, DependencyGraph.jsx, ConfigEditor/
│   ├── hooks/              # ~25 hooks: useResource.js, useResourceWatcher.js,
│   │                       # usePortForwards.js, useIngressForward.js, etc.
│   └── utils/
│       ├── resourceRegistry.js  # Central resource type definitions
│       ├── k8s-helpers.js       # Status helpers
│       ├── Logger.js            # Logging utility
│       └── formatting.js        # Date/time formatting
│
├── docs/
│   ├── ai/README.md        # THIS FILE
│   ├── architecture.md     # High-level architecture
│   ├── getting-started.md  # Setup guide
│   ├── PERFORMANCE-PLAN.md # Optimization strategies
│   └── APPLE-SILICON-OPTIMIZATIONS.md
│
└── Makefile                # dev, build, build-all, test, profile, build-pgo
```

## Key Systems

### 1. Event Coalescing (`eventcoalescer.go`)
Batches K8s watch events within 16ms windows (60fps). Deduplicates rapid updates. DELETE emits immediately. Reduces IPC overhead significantly.

### 2. Wails Communication
**Bindings** (sync): Go methods auto-exposed to JS
```go
// Go (app.go)
func (a *App) ListPods(namespace string) ([]v1.Pod, error)
```
```js
// JS
import { ListPods } from '../wailsjs/go/main/App';
const pods = await ListPods('default');
```

**Events** (async): Real-time updates
```go
// Go emits
runtime.EventsEmit(a.ctx, "pod-event", event)
```
```js
// JS listens
window.runtime.EventsOn("pod-event", callback)
```

### 3. Resource Watcher Pattern
Watchers use reference counting. Start on first subscriber, cleanup 5s after last unsubscribe.

### 4. Port Forwarding
Persistent configs stored in user settings. Modes: favorites, all, none. Supports pod and service forwards. HTTPS for browser.

### 5. Terminal Sessions
Platform-specific: PTY on Unix, conpty on Windows. WebSocket-based with resize support. Session IDs for lifecycle management.

### 6. HTTP Protocol Management
Supports HTTP/1.1 vs HTTP/2 selection. Avoids HTTP/2 flow control bottlenecks. Connection warmup/cooldown for performance.

### 7. AI Integration (`pkg/ai/`)
Integrates AI assistants (Claude) for K8s operations. MCP server provides tools for AI interactions. Supports headless and server modes for programmatic access.

## Context Providers

```js
// K8sContext
const { contexts, currentContext, switchContext, namespaces,
        selectedNamespaces, setSelectedNamespaces, crds } = useK8s();

// UIContext
const { activeView, setActiveView, openTab, closeTab, bottomTabs,
        openModal, closeModal, navigateWithSearch } = useUI();

// ConfigContext
const { config, updateConfig, portForwards, savePortForward } = useConfig();

// ThemeContext
const { theme, setTheme, themes } = useTheme();

// AIChatContext
const { messages, sendMessage, isLoading } = useAIChat();
```

## Data Fetching Pattern

All resource hooks follow:
```js
export const useResource = (currentContext, selectedNamespaces, isVisible) => {
  const [items, setItems] = useState([]);

  useEffect(() => {
    if (!isVisible) return;
    const data = await ListResources(namespace);
    setItems(data);

    StartResourceWatcher(namespace);
    window.runtime.EventsOn("resource-event", handleEvent);
  }, [currentContext, selectedNamespaces, isVisible]);

  return { items, loading, error };
};
```

## Feature Module Pattern

Each resource type (`features/[category]/[resource]/`):
- `[Resource]List.jsx` - List view with ResourceList component
- `use[Resource]Actions.jsx` - Edit, delete, view handlers
- `[Resource]ActionsMenu.jsx` - Context menu (optional)

## Adding New Resource

1. **Backend** (`pkg/k8s/client.go`): Add List/Get/Update/Delete methods
2. **Expose** (`app_[domain].go`): Wrap client methods, add watcher (use existing domain file or create new)
3. **Generate**: `wails generate module`
4. **Hook** (`hooks/use[Resource].js`): Copy pattern from existing
5. **Feature** (`features/[category]/[resource]/`): List + Actions
6. **Register** (`utils/resourceRegistry.js`): Add resource config
7. **Route** (`App.jsx`): Add case in renderContent
8. **Sidebar** (`Sidebar.jsx`): Add navigation item

## Critical Files Quick Reference

| Task | File(s) |
|------|---------|
| Add K8s operation | `pkg/k8s/client.go` + `app_[domain].go` |
| Add view/feature | `App.jsx` + `Sidebar.jsx` + `features/` |
| Add context state | Relevant `context/*.jsx` |
| Add shared component | `components/shared/` |
| Configure resource | `utils/resourceRegistry.js` |
| Theme customization | `app_themes.go` + `ThemeContext.jsx` |
| Port forward logic | `app_portforward.go` + `hooks/usePortForwards.js` |
| Terminal behavior | `app_terminal.go` + `pkg/terminal/` + `Terminal.jsx` |
| Log streaming | `app_logs.go` + `log-viewer/` |
| Helm operations | `app_helm.go` + `pkg/helm/` |
| Dependency graph | `pkg/k8s/dependencies.go` + `DependencyGraph.jsx` |
| AI integration | `app_ai.go` + `pkg/ai/` + `AIChatContext.jsx` |
| MCP server | `pkg/mcp/server.go` |
| Watcher infrastructure | `app_watchermgr.go` + `app_watchers.go` |
| Performance metrics | `app_perfmetrics.go` |

## Build Commands

```bash
make dev              # Development with hot-reload
make build            # Current platform
make build-release    # Optimized portable
make build-all        # All platforms
make test             # Frontend tests
make profile          # Collect PGO profile
make build-pgo        # Build with PGO
```

## Performance Notes

- Event coalescing: 16ms batches
- Reference-counted watchers
- Request-scoped caching in dependency graphs
- HTTP/1.1 option for parallelism
- Sequence tracking prevents stale updates
- PGO available (5-15% improvement)
- Apple Silicon runtime tuning (GOGC=150, GOMEMLIMIT=2GB)

## Supported Resources

**Workloads**: Pods, Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs, CronJobs
**Cluster**: Nodes, Namespaces, Events, Metrics, PriorityClasses, Webhooks
**Config**: ConfigMaps, Secrets, HPAs, PDBs, ResourceQuotas, Leases, LimitRanges
**Storage**: PVs, PVCs, StorageClasses, CSI Drivers, CSI Nodes
**Network**: Services, Ingresses, NetworkPolicies, Endpoints, EndpointSlices, IngressClasses
**Access Control**: Roles, ClusterRoles, RoleBindings, ClusterRoleBindings, ServiceAccounts
**Custom**: CRDs and custom resource instances
**Helm**: Releases and repositories
