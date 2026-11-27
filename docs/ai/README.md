# Kubikles AI Reference

Quick-access reference for AI assistants. This document eliminates the need to re-scan the codebase.

> **Required**: This document is the single source of truth for the file index. When adding/removing files in `pkg/k8s/`, `frontend/src/features/`, `frontend/src/hooks/`, `frontend/src/context/`, or root `.go` files, update **this document** in the same session. `.claude/rules/kubikles-context.md` and `.claude/skills/kubikles-ref/SKILL.md` reference this doc rather than duplicating it, so there is only one index to maintain.

> **DO NOT** store line counts, file sizes, file counts, or any metrics that become stale after code changes.

## Identity

**Kubikles** - Lightweight, high-performance desktop Kubernetes client. Go+React via Wails framework. Alternative to Lens.

## Tech Stack

| Layer | Tech |
|-------|------|
| Desktop | Wails v2 |
| Backend | Go 1.24+, client-go |
| Frontend | React 18, TypeScript, Vite, TailwindCSS |
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
│   ├── app_perfmetrics.go  # Performance metrics (includes shared-store stats)
│   ├── app_store.go        # Shared store wiring: client provider, sinks, PodNodeSource install, reset/resync/close hooks
│   ├── app_bulk.go         # Bulk transport: GetBulkInfo (token, setting), ReportBulkProbe, BulkBenchIPC, handler construction
│   ├── app_eventdedupe.go  # Opt-in dedupe of resourceVersion-only MODIFIED watch events (setting legacyEventDedupe, SetLegacyEventDedupe)
│   ├── app_views.go        # View protocol bindings (OpenView, UpdateView, CloseView, SetViewActive, GetViewWindow, GetViewRefs, GetViewFacet, SetViewSelection, GetResourceObject(s), GetViewsInfo), object lookup (warm shard, else live GET), view route provider, server-mode disconnect hook
│   ├── app_overlays.go     # Metrics overlays of views: poller wiring over rows.OverlayHub, preferred-source rules of the legacy hooks, SetMetricsOverlayConfig
│   ├── app_pods.go         # Pod operations
│   ├── app_deployments.go  # Deployment operations
│   ├── app_services.go     # Service operations
│   ├── app_helm.go         # Helm releases/repos (//go:build helm)
│   ├── app_helm_stub.go    # Helm stubs for lite builds (//go:build !helm)
│   ├── app_logs.go         # Log streaming
│   ├── app_terminal.go     # Terminal sessions
│   ├── app_ai.go           # AI assistant
│   ├── app_issuedetector.go # Issue detection Wails bindings
│   └── ...                 # (see rule file for complete list)
├── dispatch_gen.go         # Generated method dispatch (go generate)
├── generate.go             # go:generate directive for dispatcher
├── runtime_tuning.go       # GC percent + soft memory limit plan (env GOGC/GOMEMLIMIT win)
├── runtime_desktop.go      # isHeadlessBuild = false (//go:build !headless)
├── runtime_headless.go     # isHeadlessBuild = true (//go:build headless)
├── runtime_mem_*.go        # Physical memory probe per OS (darwin, linux, windows, other)
├── perfmem.go              # Dashboard memory stats from runtime/metrics (no stop-the-world)
├── viewnotifier.go         # ViewNotifier: one batched `view-updates` event per frame, following the EventCoalescer cadence and background mode
├── eventcoalescer.go       # Frame-window event batching, ordered async delivery, background cadence
├── logcoalescer.go         # Log streaming batching (per-stream buffers, background cadence)
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
│   │   ├── client.go           # Core client struct, config, lifecycle, per-context clientsets
│   │   ├── contexts.go         # Context DTOs + kubeconfig CRUD (get/delete/rename/update)
│   │   ├── watch.go            # WatchResource + WatchCRD
│   │   ├── apply.go            # ApplyYAML + kindToResource map
│   │   ├── configmaps.go       # ConfigMaps & Secrets CRUD
│   │   ├── csi.go              # CSI Drivers & CSI Nodes
│   │   ├── customresources.go  # CRDs + custom resource CRUD
│   │   ├── daemonsets.go       # DaemonSet operations
│   │   ├── dependencies.go     # Dependency graph types + GetResourceDependencies/ExpandDependencyNode entry points
│   │   ├── deps_cache.go       # Request-scoped resourceCache + getters
│   │   ├── deps_graph.go       # Graph aggregation + node/edge primitives
│   │   ├── deps_pod.go         # Pod dependency resolution (owner refs, volumes, PVC/PV, container refs)
│   │   ├── deps_controllers.go # Workload resolvers (deployment, statefulset, daemonset, replicaset, job, cronjob)
│   │   ├── deps_resources.go   # Remaining resource resolvers + selector/find helpers
│   │   ├── deployments.go      # Deployment operations
│   │   ├── diff.go             # Resource diff/comparison
│   │   ├── events.go           # Event operations
│   │   ├── fileops.go          # File operations for K8s resources
│   │   ├── flowtimeline.go     # Flow timeline for resource events
│   │   ├── ingresses.go        # Ingress & IngressClass operations
│   │   ├── jobs.go             # Job & CronJob operations
│   │   ├── metrics.go          # Metrics-server node/pod metrics
│   │   ├── metrics_snapshot.go # Single-flight short-TTL cache of cluster-wide Pod/Node/PodMetrics lists
│   │   ├── multilog.go         # Multi-resource log streaming
│   │   ├── namespaces.go       # Namespace operations + resource counts
│   │   ├── networkpolicies.go  # NetworkPolicies, HPAs, PDBs, Quotas, LimitRanges, Endpoints
│   │   ├── debugcluster_policy.go  # (debugcluster tag) service accounts, quotas, limit ranges, network policies, HPAs, PDBs and leases of each fake debug namespace
│   │   ├── debugcluster_network.go # (debugcluster tag) ingresses, endpoints, endpoint slices, roles and role bindings of each fake debug namespace
│   │   ├── debugcluster_metrics.go # (debugcluster tag) synthetic pod and node metrics when KUBIKLES_DEBUG_METRICS=1
│   │   ├── nodes.go            # Node operations
│   │   ├── pods.go             # Pod listing, CRUD, eviction, owner resolution
│   │   ├── podlogs.go          # Pod log retrieval + streaming (single/all containers, all pods)
│   │   ├── podlogs_history.go  # Bounded history paging (first page / page before a cutoff) without full download
│   │   ├── listpages.go        # List page observer + out-of-band ListOutcome (resourceVersion anchor)
│   │   ├── prometheus.go       # Prometheus types + detection
│   │   ├── prometheus_query.go # Raw query plumbing + range helpers
│   │   ├── prometheus_history.go # Pod/controller/namespace/node metrics-history builders
│   │   ├── prometheus_batch.go # Bounded-concurrency batch of independent range queries
│   │   ├── resources.go        # ResourceDef table (GVR, list, watch per built-in type); behind WatchResource and the store
│   │   ├── table.go            # Server-side Table list (ListTable), RESTClientOf, ResourcePath; shared by secrets list and the store
│   │   ├── storeread.go        # StoreReader seam, WithStoreRead, StoreList[T], NewClientWithClientset
│   │   ├── store_clients.go    # KubeClient/DynamicClient/MetadataClient accessors for the store, APIPodNodeSource
│   │   ├── rbac.go             # RBAC permission checking
│   │   ├── rbacresources.go    # Roles, ClusterRoles, Bindings CRUD
│   │   ├── replicasets.go      # ReplicaSet operations
│   │   ├── scheduling.go       # PriorityClasses & Leases
│   │   ├── serviceaccounts.go  # ServiceAccount operations
│   │   ├── services.go         # Service operations + backing pods
│   │   ├── statefulsets.go     # StatefulSet operations
│   │   ├── storage.go          # PVC, PV, StorageClass operations
│   │   └── webhooks.go         # Validating & Mutating webhook configs
│   ├── terminal/
│   │   ├── manager.go      # Session lifecycle
│   │   ├── outputcoalescer.go # PTY output batching, UTF-8 safe, immediate first write after idle
│   │   ├── session_unix.go # Unix/macOS PTY
│   │   └── session_windows.go # Windows conpty
│   ├── helm/
│   │   ├── client.go       # Helm operations (//go:build helm)
│   │   ├── client_stub.go  # Stub client for lite builds (//go:build !helm)
│   │   ├── types.go        # Pure data types (no build tag)
│   │   ├── oci.go          # OCI registry (//go:build helm)
│   │   └── repo.go         # Repository management (//go:build helm)
│   ├── bulk/               # Same-origin GET bulk transport under /_kubikles/v1/ (no Wails imports)
│   │   ├── handler.go      # NewHandler, call route (allowlisted reads, NDJSON for slices), Chain for the Wails asset middleware
│   │   ├── allowlist.go    # Read-method allowlist (mirrored by frontend/src/lib/bulk/bulkMethods.ts)
│   │   ├── guards.go       # Marker + token + Host checks, no-store, GET/HEAD only, never 304, no CORS
│   │   ├── stream.go       # ChunkWriter: 64KB chunks, per-chunk write deadline, write error means cancel
│   │   ├── gzip.go         # Flushing gzip writer (server mode only)
│   │   ├── host.go         # Host checker for server mode
│   │   └── probe.go        # probe/ping, stream, bench, bytes routes used by the in-app probe
│   ├── store/              # Shared list+watch shards (no package main imports); read by row views, the PodNodeSource seam and the warm readers
│   │   ├── doc.go          # Package overview
│   │   ├── key.go          # Key{Context,Type,Namespace}, CRD type strings
│   │   ├── entry.go        # Immutable Entry, Change types
│   │   ├── source.go       # Mode, Source interface, TableRow, ErrModeUnsupported
│   │   ├── source_list.go  # Shared paged LIST driver and item extraction
│   │   ├── source_typed.go # Typed clientset source over k8s.ResourceDef (works on kubernetes/fake)
│   │   ├── source_dynamic.go # Custom resources as unstructured
│   │   ├── source_meta.go  # Metadata client source and in-process metadata conversion fallback
│   │   ├── source_table.go # Table list and Table watch stream decoder
│   │   ├── shard.go        # Reflector (backoff, premature close, stale RV, forbidden, diffing relist, polling) and entry maps
│   │   ├── subscribe.go    # Bounded subscriber queues with reset-on-overflow
│   │   ├── manager.go      # Leases, linger, cluster-wide-serves-namespaced, Peek, Lookup (one object from a warm full shard), Pin, PinExisting (never creates a shard), Reset, Resync, byte budget, Stats
│   │   ├── podnode.go      # k8s.PodNodeSource backed by warm shards
│   │   ├── reader.go       # Reader: k8s.StoreReader/StoreCounter over warm shards (dependency graphs, issue detector, tools, namespace counts)
│   │   ├── source_prune.go # Full-object custom resource source that keeps only metadata and the JSONPath roots the columns read
│   │   └── clock.go        # Injectable clock
│   ├── watchevents/
│   │   └── watchevents.go  # watcher-status / watcher-error payload types shared by watcher manager and store
│   ├── rows/               # Windowed list engine over store shards (no package main imports); migrated type == registered Spec
│   │   ├── spec.go         # Spec, Column, Row, Preset, Derived, Overlay, SchemaVersion
│   │   ├── registry.go     # Register (validation), Lookup, Types
│   │   ├── spec_common.go  # Shared metadata columns and common search fields
│   │   ├── spec_crd.go     # GenericCRDSpec: row spec of any custom resource from its printer columns (JSONPath via client-go, Table cells when present); CRDDescriptor, CRDResolver
│   │   ├── spec_namespaces.go, spec_configmaps.go, spec_secrets.go, spec_events.go, spec_priorityclasses.go, spec_ingressclasses.go, spec_storageclasses.go, spec_webhooks.go (validating and mutating), spec_csidrivers.go, spec_csinodes.go, spec_persistentvolumes.go, spec_clusterroles.go, spec_clusterrolebindings.go, spec_hpas.go, spec_pdbs.go, spec_resourcequotas.go, spec_limitranges.go, spec_leases.go, spec_networkpolicies.go, spec_serviceaccounts.go, spec_roles.go, spec_rolebindings.go, spec_endpoints.go, spec_endpointslices.go, spec_services.go, spec_ingresses.go, spec_replicasets.go, spec_jobs.go, spec_daemonsets.go, spec_persistentvolumeclaims.go, spec_deployments.go, spec_statefulsets.go, spec_pods.go, spec_nodes.go  # One spec per migrated type; spec_workload_common.go holds the controller owner reference, pod template and workload search-field helpers the workload specs share
│   │   ├── types.go        # ViewSpec, ViewPatch, ViewState, ViewWindow, ViewNotice, Facet and the error values
│   │   ├── values.go       # JS-compatible comparison (UTF-16 order), cell equality, JS key order and JSON.stringify lengths
│   │   ├── sort.go         # sortItems port: primary key, newest-first, name collator tie-breaks
│   │   ├── columnfilter.go # compileColumnFilters port: select, regex and numeric column filters
│   │   ├── core.go         # Per-view engine: scope rows, sorted index, facets, incremental upsert/remove, incremental overlay and derived-column application (applyOverlay, applyPods, refreshRows)
│   │   ├── podindex.go     # Derived-column engine: podIndex (namespace plus label inverted index of pods, effective pod status port), PodRef, match by matchLabels
│   │   ├── podfeed.go      # A workload view's pod feed: leases on the pod shards of its namespaces, subscriptions that keep the index current, onPods fold into the view
│   │   ├── transitions.go  # ViewSpec.NotifyOn "pods": Transition log (restart, deleted, added) merged per pod between notices, carried in ViewNotice.Transitions
│   │   ├── derived_golden_test.go, derived_property_test.go, derived_view_test.go, transitions_test.go  # Derived cells per recorded pod list, incremental-equals-brute-force property test, workload view following pods, pod transitions
│   │   ├── overlay.go      # OverlayHub: latest snapshot per context and source, delta between snapshots, subscriptions with demand callbacks
│   │   ├── overlay_metrics.go  # Pod, node and namespace metrics: record layouts, the percentage transforms of the legacy hooks, overlay cells and filter values
│   │   ├── overlay_poller.go   # OverlayPoller: polls a source only while a view wants it and the window is visible
│   │   ├── overlay_golden_test.go, overlay_view_test.go, overlay_poller_test.go, overlay_bench_test.go  # Overlay golden parity per metrics state, incremental-equals-rebuild property tests, poller gating, large-view tick benchmarks
│   │   ├── view.go         # View: leases and subscriptions, rebuild off-lock, windows with seq deltas, facets, selection, notices
│   │   ├── manager.go      # ViewManager: Open/Update/Close, lease expiry, caps, Reset, client disconnect, namespace planning
│   │   ├── planner.go      # chooseNamespaceScope port (cluster-wide vs per-namespace shards)
│   │   ├── routes.go       # GET /_kubikles/v1/views/{id}/window|refs|rows and /objects/{type} (bulk.RouteProvider)
│   │   ├── tsgen.go        # Renders frontend/src/generated/rowSchemas.ts from the registry
│   │   ├── batcha_integration_test.go  # Golden fixtures served through a fake clientset, the real store and real views (live-data check for types the debug cluster cannot hold; also scopes namespaced types by namespace)
│   │   ├── testdata/golden/# Parity files written by the legacy frontend code (see frontend/src/test/golden)
│   │   └── query/          # Search language: parser.go (queryParser.ts port), engine.go (filterEngine.ts port), jsregex.go (RE2 first, regexp2 ECMAScript fallback), jsstring.go (JavaScript string semantics: UTF-16 order, whitespace set, pattern rewriting for JS parity)
│   ├── compressedassets/
│   │   └── handler.go      # Wails middleware + gzip-aware file server (hashed assets immutable, index revalidated)
│   ├── ai/                 # AI integration
│   │   ├── claude_cli.go   # Claude CLI integration
│   │   ├── manager.go      # AI session manager
│   │   └── provider.go     # Provider abstraction
│   ├── issuedetector/       # Cluster issue detection engine
│   │   ├── types.go         # Core types (Finding, ScanResult, ScanProgress, RuleInfo)
│   │   ├── rule.go          # Rule interface and base helper
│   │   ├── resourcecache.go # Parallel resource fetching with typed getters
│   │   ├── engine.go        # ScanEngine orchestration
│   │   ├── yamlloader.go    # YAML rule file parser (6 check types)
│   │   ├── rules_builtin.go # Built-in rule registration
│   │   ├── rules_networking.go    # NET001-NET005
│   │   ├── rules_workloads.go     # WRK001-WRK004
│   │   ├── rules_storage.go       # STR001-STR002
│   │   ├── rules_security.go      # SEC001-SEC002
│   │   ├── rules_config.go        # CFG001-CFG002
│   │   ├── rules_deprecation.go   # DEP001-DEP005
│   │   └── engine_test.go         # Unit tests
│   ├── debug/              # Structured debug logging
│   │   └── logger.go       # Debug logger with categories, emits events
│   ├── mcp/                # MCP server
│   │   └── server.go       # MCP protocol implementation
│   ├── tools/              # Tool registry for AI
│   │   ├── registry.go     # Tool registration
│   │   ├── run_command.go  # Controlled shell command execution with prefix allowlist
│   │   ├── tools.go        # Tool registry (AllToolDefs), dispatch (CallTool), arg helpers
│   │   ├── tools_resources.go   # Resource tools: logs, yaml, list, events, describe
│   │   ├── tools_metrics.go     # Metrics & dependency tools
│   │   ├── tools_crd.go         # CRD & custom-resource tools
│   │   ├── tools_diagnostics.go # Flow timeline, multi-pod logs, diff, RBAC
│   │   └── tools_format.go      # Formatting & string helpers
│   ├── server/             # Server mode
│   │   ├── api.go          # REST API handlers
│   │   ├── client.go       # Per-WebSocket-client bounded send queue + writer goroutine
│   │   ├── gzip.go         # Gzip for large API responses when the client accepts it
│   │   └── server.go       # HTTP server (uses MethodCaller interface), encode-once fan-out
│   ├── events/             # Event system
│   │   └── emitter.go      # Event emitter
│   ├── hosts/              # Platform-specific hosts file
│   ├── certviewer/         # Certificate inspection
│   └── crashlog/           # Crash logging
│
├── cmd/
│   ├── gen-dispatcher/
│   │   └── main.go         # Code generator for method dispatch switch
│   └── gen-rows/
│       └── main.go         # Writes frontend/src/generated/rowSchemas.ts from the pkg/rows registry
│
├── scripts/
│   └── compress-dist.sh    # Post-build gzip compression of frontend dist
│
├── frontend/src/
│   ├── App.tsx             # Root component, providers, view routing
│   ├── main.tsx            # Entry, Monaco config
│   ├── fonts/              # Bundled font CSS: core.css (Inter, JetBrains Mono, eager), dm-sans.css (lazy)
│   ├── context/
│   │   ├── K8sContext.tsx      # K8s state (contexts, namespaces, CRDs)
│   │   ├── UIContext.tsx       # UI state (tabs, modals, panels)
│   │   ├── ConfigContext.tsx   # User settings, port forwards
│   │   ├── ThemeContext.tsx    # Active theme
│   │   ├── MenuContext.tsx     # Context menus
│   │   ├── DebugContext.tsx    # Debug logging
│   │   ├── NotificationContext.tsx # Toast notifications
│   │   └── AIChatContext.tsx   # AI chat integration
│   ├── features/
│   │   ├── workloads/      # pods/, deployments/, statefulsets/, daemonsets/,
│   │   │                   # replicasets/, jobs/, cronjobs/; workloadSemantics.ts (controller
│   │   │                   # owner reference and pod template reads shared by the migrated lists),
│   │   │                   # ControlledBy.tsx (the Controlled By link of the replica set and job lists)
│   │   ├── cluster/        # nodes/, namespaces/, events/, metrics/, webhooks/,
│   │   │                   # priorityclasses/, topology/
│   │   │   └── topology/
│   │   │       ├── NodeTopology.tsx          # Main topology canvas with React Flow
│   │   │       ├── TopologyNodeComponent.tsx # Custom React Flow node for K8s nodes
│   │   │       └── topologyUtils.ts         # Layout and pod grouping utilities
│   │   ├── config/         # configmaps/, secrets/, hpas/, pdbs/, resourcequotas/,
│   │   │                   # leases/, limitranges/
│   │   ├── storage/        # pv/, pvc/, storageclass/, csidrivers/, csinodes/
│   │   ├── network/        # services/, ingresses/, networkpolicies/, endpoints/,
│   │   │                   # endpointslices/, ingressclasses/
│   │   ├── access-control/ # roles/, clusterroles/, rolebindings/,
│   │   │                   # clusterrolebindings/, serviceaccounts/
│   │   ├── customresources/ # CRDs and custom instances
│   │   ├── helm/           # Helm releases, repos, OCI
│   │   ├── diagnostics/    # Resource comparison, diagnostics, issue detection, ScratchPad.tsx (notepad)
│   │   └── portforwards/   # Port forward management UI
│   ├── components/
│   │   ├── layout/         # Sidebar.tsx, BottomPanel.tsx, AIPanel.tsx,
│   │   │                   # MessageBubble.tsx (one bubble per turn),
│   │   │                   # ToolCallCard.tsx (inline collapsible tool call), AIMarkdown.tsx
│   │   └── shared/         # ResourceList.tsx, YamlEditor.tsx, LogViewer.tsx,
│   │       │               # Terminal.tsx, DependencyGraph.tsx, ConfigEditor/
│   │       ├── resourceListPipeline.ts # Pure sort/filter/unique-value logic for ResourceList
│   │       ├── ResourceListHeader.tsx  # Memoised sortable/reorderable header
│   │       ├── ResourceListRows.tsx    # Memoised row and cell rendering
│   │       ├── LiveAge.tsx             # Age cell that re-renders itself when its text is due to change (per second while seconds show)
│   │       ├── MenuBound.tsx           # Row actions menu bound to the open-menu store
│   │       └── log-viewer/
│   │           ├── logBuffer.ts        # Bounded append-optimised log store, frame-throttled snapshots
│   │           ├── followPin.ts        # Whether a followed log view stays pinned to its end (released and restored by user scrolling)
│   │           ├── streamLedger.ts     # Ids of the streams this page opened, kept across a reload so the next page stops leftovers (desktop)
│   │           ├── listReady.ts        # Cover state while a remounted log list measures its first rows
│   │           └── TabActiveContext.ts # Whether the hosting bottom-panel tab is shown
│   ├── features/cluster/nodes/NodeMetricCells.tsx    # Node metric bars and their store-backed (legacy) and view-row cells
│   ├── features/cluster/nodes/NodeCells.tsx, bulkCordon.ts, semantics.ts, viewColumns.tsx    # Conditions and taints cells; sort/filter semantics (createNodeSemantics, metrics read through a lookup); windowed columns
│   ├── features/cluster/namespaces/NamespaceMetricCells.tsx    # Namespace metric bars and their store-backed and view-row cells
│   ├── features/workloads/pods/PodMetricCells.tsx    # Pod metric bars and their store-backed (legacy) and view-row cells
│   ├── features/workloads/pods/PodCells.tsx, evictable.ts, semantics.ts, viewColumns.tsx    # Container squares, status, controller link; sort/filter semantics (createPodSemantics, metrics read through a lookup); windowed columns
│   ├── features/workloads/podDerivation.ts, PodSquares.tsx    # Legacy pod-to-workload matching (deploymentPodMaps, podRefsText) shared with the golden generator; the Pods cell squares of a windowed workload row
│   ├── features/workloads/deployments/semantics.ts, viewColumns.tsx, features/workloads/statefulsets/semantics.ts, viewColumns.tsx    # createDeploymentSemantics / createStatefulSetSemantics (the Pods column reads a pod lookup); windowed columns
│   ├── hooks/useViewOverlay.ts      # useMetricsOverlayConfig (sends source and poll interval to the backend), useOverlayAvailability (metricsAvailable of a view as a store)
│   ├── hooks/              # useResource.tsx, useResourceWatcher.tsx, useStableCallbacks.ts,
│   │                       # usePortForwards.tsx, useIssueDetector.tsx, etc.
│   ├── constants/
│   │   ├── menuStructure.ts      # Sidebar menu items, default sections, reconciliation
│   │   └── sidebarLayoutUtils.ts # Pure sidebar layout manipulation functions
│   ├── types/
│   │   └── k8s.ts          # TypeScript K8s resource type definitions
│   ├── generated/rowSchemas.ts # Generated by cmd/gen-rows: per migrated type the columns, filter types, predefined values, search fields and cell types
│   ├── test/golden/        # generateGolden.test.ts runs the legacy list code over fixtures.ts and writes/asserts pkg/rows/testdata/golden/*.json (GOLDEN=1 rewrites); goldenIo.js reads and writes the golden files from the test; rowSchemas.test.ts checks the generated schema against the legacy modules
│   ├── lib/bulk/           # Bulk transport client: transport.ts (fetch + NDJSON + per-call fallback),
│   │                       # probe.ts (capability probe, decision table, window.__kubiklesProbe),
│   │                       # bulkMethods.ts (allowlist mirror), types.ts (shared caps and info types), probeWorker.js (probe worker source)
│   ├── lib/views/          # View client for windowed lists: protocol.ts (wire types), viewClient.ts (pure, injected transport and notices),
│   │                       # transport.ts (bulk GET with binding fallback, shared notice feed), rowItem.ts, schema.ts (accessors over generated/rowSchemas.ts),
│   │                       # viewColumns.ts (defineViewColumns, column-filter wire conversion), viewRegistry.ts (every list type: on the view path or the reason it is not; viewRegistry.test.ts fails for a list that is neither), availability.ts (setting + backend + degraded kinds), testSupport.ts,
│   │                       # workerProtocol.ts (worker messages, packed refs), viewWorkerCore.ts (pure WorkerCore: view client + fetch transport inside the worker),
│   │                       # viewWorker.ts (worker entry), viewWorkerLoader.ts (inline Blob worker via ?worker&inline), viewHost.ts (main-thread end: rpc, notice forwarding, watchdog),
│   │                       # viewClientProxy.ts (ViewHandle over the worker, row mirror, per-frame apply), viewParking.ts (keeps the view of a list just left open briefly so a return reuses it), viewClientFactory.ts (level choice, watchdog degradation, fetchBulkInWorker),
│   │                       # workerTestSupport.ts (in-memory fake worker)
│   ├── features/customresources/instances/  # CustomResourceList (legacy and windowed branches), viewColumns.tsx (columns from the dynamic schema), renderColumnValue.tsx
│   ├── components/shared/TransportSection.tsx  # Transport tab of the PerformancePanel (bulk caps, probe, view worker level)
│   ├── components/shared/StoreViewsSection.tsx  # Store tab of the PerformancePanel (shards, views, rates); storeViewsStats.ts holds the pure helpers
│   ├── hooks/useResourceView.tsx   # Opens/closes a view per context+type, patches scope changes, visibility, polling, load state
│   ├── hooks/useRemoteOrLegacy.tsx # Seam used by hooks/resources/index.ts: Go view when available, legacy hook otherwise
│   ├── hooks/useViewObject.ts      # useWithFullObject: fetch the full object at click time for actions that need spec; useAllViewObjects: every object of a view (node topology)
│   ├── features/*/viewColumns.tsx  # Presentation-only columns of the windowed pilot lists (pods, nodes, namespaces, configmaps, secrets, events, priorityclasses, ingressclasses, storageclasses, validating and mutating webhook configurations, csidrivers, csinodes, persistentvolumes, clusterroles, clusterrolebindings, hpas, pdbs, resourcequotas, limitranges, leases, networkpolicies, serviceaccounts, roles, rolebindings, endpoints, endpointslices, services, ingresses); viewColumnTestSupport.ts holds the shared render helpers and the cell-read recorder of the view-column tests
│   └── utils/
│       ├── resourceRegistry.ts  # Central resource type definitions
│       ├── backendVisibility.ts # Reports window visibility to the backend (desktop only)
│       ├── windowVisibility.ts  # Shared visibility source (useSyncExternalStore compatible)
│       ├── valueStore.ts        # External value store with selector subscriptions (cells read, columns stay stable)
│       ├── metricsTransform.ts  # Pure pod, node and namespace metrics transforms (legacy hooks, golden generator) and the record encoding of view cells
│       ├── frame.ts             # Animation frame raced against a timer, for work that must not stall in an inactive window
│       ├── columnSemantics.ts   # Shape of the pure per-column sort/filter semantics modules (features/<area>/<type>/semantics.ts)
│       ├── fontLoader.ts        # Lazy loading of optional UI fonts
│       ├── terminalResize.ts    # Leading + trailing throttled terminal fit
│       ├── k8s-helpers.ts       # Status helpers
│       ├── Logger.ts            # Logging utility
│       ├── aiChatBlocks.ts      # Pure reducer for AI turn blocks (text + tool cards)
│       ├── aiChatBlocks.test.ts # Unit tests for the block reducer
│       └── formatting.ts        # Date/time formatting
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
Batches K8s watch events within frame windows (16ms by default, backing off adaptively under load). Deduplicates rapid updates (latest wins per resource). DELETE emits immediately. Flushed batches go through an ordered asynchronous delivery queue, so a slow bridge never blocks watch loops; when the queue is full, events keep coalescing in the pending map instead of piling up. While the window is hidden (`SetWindowVisible`, reported by `frontend/src/utils/backendVisibility.ts`) both the event and log coalescers flush at a slow cadence and flush everything on return. In server mode the frontend does not report visibility, since one hidden tab must not slow updates for other viewers. Terminal output has its own coalescer (`pkg/terminal/outputcoalescer.go`).

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

**List/watch handoff**: list calls take a request ID; IDs prefixed `page-list-` stream each page as a `list-page` event and the final array may then be empty. `GetListOutcome(requestId)` returns the list's resourceVersion, whether it was streamed and the loaded count. The frontend then calls `SubscribeResourceWatcherFrom` / `SubscribeCRDWatcherFrom` with that resourceVersion so the watch starts exactly after the list (no replay of existing objects as ADDED). When no outcome is available it falls back to the unanchored `Subscribe*` calls. List results have `metadata.managedFields` stripped.

**Wire format**: the typed clientset talks Kubernetes protobuf by default; set `KUBIKLES_DISABLE_PROTOBUF=1` for proxies or gateways that only pass JSON. Watch events are encoded directly to JSON rather than via an intermediate map.

### 4. Port Forwarding
Persistent configs stored in user settings. Modes: favorites, all, none. Supports pod and service forwards. HTTPS for browser.

### 5. Terminal Sessions
Platform-specific: PTY on Unix, conpty on Windows. WebSocket-based with resize support. Session IDs for lifecycle management. Output is coalesced (first bytes after idle go out immediately, bursts are batched, multi-byte runes are never split); the frontend throttles resize fits.

### 6. HTTP Protocol Management
Supports HTTP/1.1 vs HTTP/2 selection. Avoids HTTP/2 flow control bottlenecks. Connection warmup/cooldown for performance.

### 7. AI Integration (`pkg/ai/`)
Integrates AI assistants (Claude) for K8s operations. MCP server provides tools for AI interactions. Supports headless and server modes for programmatic access.

### 8. Issue Detector (`pkg/issuedetector/`)
Rule-based cluster scanning engine. Built-in Go rules cover networking (NET), workloads (WRK), storage (STR), security (SEC), config (CFG), and deprecation (DEP) categories. YAML-based custom rules support 6 check types. Engine uses parallel resource caching, emits progress events during scans, and returns findings with severity/category/remediation.

### 9. Log Viewer Store (`frontend/src/components/shared/log-viewer/`)
Streamed lines are appended in place to a bounded `logBuffer` (O(batch) per append, oldest lines dropped past the cap) and published to React as immutable snapshots at most once per frame. Line ids are stable (`firstId + position`) so search and rendering survive trimming. Search is incremental over appended lines. Entering or leaving the filtered search list remounts the scroller, which stays hidden until it has measured rows; `listReady.ts` covers the pane with a loading state for that window. A hidden bottom-panel tab (`TabActiveContext`) skips rendering while streaming continues. History paging uses `pkg/k8s/podlogs_history.go` (first page, or page before a cutoff) instead of downloading the whole log. While following, `followPin` decides whether the view stays at the end: only user scrolling releases it, because the list's own at-bottom signal turns false during bursts.

### 9b. Bulk Transport (`pkg/bulk/`, `frontend/src/lib/bulk/`)
Heavy read bindings (`List*`, `Get*Yaml`, history pages, pod/node metrics; see `pkg/bulk/allowlist.go`) can travel as same-origin `GET /_kubikles/v1/call/{Method}?args=<json array>` instead of the Wails bridge or `/api/call`. Slice results arrive as NDJSON, everything else as `{"data":...}` / `{"error":...}`. Desktop mounts the handler in front of the asset middleware (`bulk.Chain` in `main_desktop.go`, no gzip, no 304); server mode mounts it on the mux with gzip, a Host check (`KUBIKLES_ALLOWED_HOSTS`) and per-chunk write deadlines. Every request needs `X-Kubikles: 1` and `X-Kubikles-Token` (per-process token from `GetBulkInfo`). The adapter (`lib/wailsjs-adapter/go/main/App.ts`) routes allowlisted methods through `tryBulkCall` only when the in-app probe passed (server mode is optimistic) and falls back to the binding per call on any transport failure; three consecutive failures pause routing. Setting `performance.bulkTransport` (auto|off) and env `KUBIKLES_DISABLE_BULK=1` switch it off. The PerformancePanel Transport tab shows caps, counters and a Run probe button.

### 9c. Windowed Views (`pkg/rows/`, `app_views.go`, `viewnotifier.go`)
A migrated resource type is one whose `rows.Spec` is registered; the spec declares columns (key identical to the TS column key, cell function, sort/filter semantics), search fields and presets once. `go generate` writes `frontend/src/generated/rowSchemas.ts` from it, and a Go test fails when the committed file is stale. Parity with the legacy TS list code is bug-for-bug and proven by golden files produced by the real legacy functions (`frontend/src/test/golden`), which the Go tests must reproduce (cells, sorts, search, column filters, unique values). The only intentional visible difference is on secrets: the view shows the data key count and has no size column, and key names need the object (`GetResourceObject`). Other differences are recorded in `Spec.Notes`. A type whose display needs more than its sort value carries a display-only column (filter none, hidden by default, always projected through `ResourceList` `viewPinnedColumns`): endpoints and endpoint slices `readyCount`, services `externalAddresses`, ingresses `tlsCount` and `defaultBackendTarget`, storage classes `isDefault`, replica sets and jobs `controllerRef` (the controller as JSON, for the Controlled By link), jobs `completionsText`, `conditionText`, `completionTime` and `imageList`, daemon sets `imageList`; the column picker lists them in view mode only. The job list keeps a legacy quirk on purpose: its name, namespace, completions and condition columns have no sort value, so sorting by them is a no-op in both paths. The job duration of a running job is evaluated when the object last changed (the display counts to now at render). The persistent volume claim list keeps its list id `pvcs` and `getRowSchema` resolves it to the `persistentvolumeclaims` schema. Cron jobs (the Next Run column is computed from the wall clock at render time) stay on the legacy path; pods, nodes and the namespace metrics columns are served through overlays and deployments and stateful sets through derived columns (both below); `lib/views/viewRegistry.ts` states why. The persistent volume list keeps its list id `pvs` (saved layouts are keyed by it) and `getRowSchema` resolves it to the `persistentvolumes` schema. `lib/views/viewRegistry.ts` states per list type whether it is on the view path and why not otherwise.

A view (`ViewManager.Open`) acquires store shard leases (cluster-wide or per namespace, chosen by the `planner.go` port of `chooseNamespaceScope`, re-planned per namespace when the cluster-wide list is forbidden), subscribes to them, and keeps a sorted index of the entries passing search and column filters, updated incrementally (binary search insert/remove, in place when only cells changed). A change that alters no projected cell (heartbeat) produces no row, no version bump and no notice. Control goes over bindings; data goes over GET routes (`routes.go`, mounted through `bulk.Deps.Routes`) with `GetViewWindow` as the binding twin. `have=<version>` returns only rows that changed after it or that the client may not hold. Tiny `view-updates` notices (counts, phase, `windowChanged`, closed reason, selection-stale uids; never rows) are batched by `ViewNotifier` at the EventCoalescer cadence. Views are leased with a client token (60s idle), capped per client and globally, closed on context switch (`Reset("context")`, notice `closed: "context"`) and, in server mode, when the WebSocket disconnects. `KUBIKLES_VIEWS=0` turns the protocol off. 
The frontend side (`frontend/src/lib/views`) is behind setting `performance.viewStore` (default off, Settings > Performance). `useRemoteOrLegacy` (wired in `hooks/resources/index.ts` for every type that has a generated schema) runs the legacy hook unchanged unless the setting is on, the backend reports the type, and the kind has not failed repeatedly in the session; the inactive path is kept idle through its visibility flag. `ResourceList` takes a `source` (view handle): the table is Virtuoso by `totalCount` with skeleton rows for windows in flight, search, column filters, sort and visible columns are pushed with `view.update`, select-all and shift ranges use the refs stream, select dropdown values come from `view.facet`, and bulk actions receive identity items (`{metadata:{uid,name,namespace}}`); anything needing spec fetches the object through `view.getObject`. Window updates reach Virtuoso rows through its `context` (epoch). Unmigrated types and the legacy branch are untouched.
Derived columns (`Spec.Derived`, `pkg/rows/podindex.go`, `podfeed.go`): the Pods column of deployments and stateful sets shows the pods the workload selects. The legacy lists pair them by namespace and equality of every `spec.selector.matchLabels` entry with the pod labels (matchExpressions ignored, no matchLabels selects nothing), not by owner references, so that relation is what the engine implements; an owner-chain index would give different answers for orphaned or re-labelled pods. A workload view leases the pods of its namespaces in full mode (`connectPods`, planned like any pod view, re-planned with the namespace selection), keeps a `podIndex` (inverted index namespace/label key/value to pods, pod status computed once per pod change by the port of `getEffectivePodStatus`) and re-derives only the workloads whose selector matches a changed pod before or after the change (`core.applyPods`; a resource-version-only update is not a change). A rebuild compares the index generation before and after it builds and re-derives everything when pods moved meanwhile. `Row.Pods()` is the one accessor cells read: the `pods` column is the count (the sort value, no filter, as legacy), the display-only `podRefs` column is JSON `[name, uid, status]` per pod, `readyText` and `imageList` carry the Ready and Image displays. `ViewState.overlays.pods.available` says whether the pod shards have listed (false: the list shows placeholder squares sized by `replicas`, like the legacy list while its pod list loads). Golden files carry a `derived` section per recorded pod list (cells, numeric values, sort orders by pod count) built by the legacy `deploymentPodMaps` and `getDeploymentPods`; Go tests prove it, incremental application equals a rebuild for every pair of lists, and a random history of pod and workload changes equals a brute-force recomputation from the raw pods.

Pod notifications (`pkg/rows/transitions.go`, `hooks/usePodNotifications.tsx`): a pod view opened with `notifyOn: ["pods"]` (a patchable ViewSpec field; only the pods type accepts it) reports `Transition`s in its notices: a restart of a pod visible in the filtered list (with the containers whose count rose), the disappearance of a visible pod, and every new pod. Initial list contents and rebuilds report nothing (like the hook re-seeding), restarts between two notices merge into one event measured from the first state, a pod that appeared and vanished between notices is never seen, and the pending list is capped. `usePodViewNotifications` feeds them through `eventsToNotifications`, the same function the legacy snapshot diff uses (restarts, then deletions per owner, then redeploys within the 30 second window, throttling and sound), so both paths produce the same messages; with viewStore off the legacy `usePodNotifications` runs unchanged. The topology diagram's node list comes from the node view in view mode (`useAllViewObjects`: refs stream plus the object route over the shared shard, ordered by name) instead of the legacy node hook; its per-node pods still come from `ListPodsForNode`, which already reads a warm store shard, and its live pod events from the pod watcher (an event stream, not a list hook).

Overlays (`pkg/rows/overlay*.go`, `app_overlays.go`): columns fed by a second source keyed by object. `Spec.Overlays` names the source (`podmetrics`, `nodemetrics`, `namespacemetrics`), the columns it feeds and the record key (`ns/name` for pods, the name for nodes and namespaces). The sources are polled by an `OverlayPoller` that reuses the legacy backend calls (`GetPodMetrics`, `GetNodeMetrics`, the Prometheus fallbacks, which pin the pod and node shards when those are already open) and the preferred-source rules of the legacy hooks (`SetMetricsOverlayConfig` carries `metrics.preferredSource` and `kubernetes.metricsPollIntervalMs`); a failed poll keeps the previous samples and reports unavailable, like the hooks. A source is polled only while at least one active view subscribed to it (the hub's demand callback) and the window is visible (`SetWindowVisible`; a hidden window skips ticks and polls once on return). Each poll publishes a snapshot to the `OverlayHub`; the hub diffs it against the previous one and each view re-derives only the overlay's columns of the rows whose record changed (`core.applyOverlay`): a tick re-sorts only while the view is sorted by a volatile column, re-evaluates membership only while a column filter targets one, and re-sorts the index once instead of moving rows one by one when a large share of rows changed. Other columns keep their positions and just get new cells; the row `seq` advances only for rows whose cells changed, so delta windows stay small. `ViewState.overlays` (and the notice) carries each source's availability (`null` until it answered, `false` when metrics are unavailable): this is the `metricsAvailable` state behind the N/A cells. Per row a metrics sample travels as one display-only `metrics` string cell (the record's numbers joined with commas, "" without a sample) beside the sort cells `cpu` and `memory` (committed percentage, -1 without a sample) whose numeric filters read the usage percentage. Golden files carry per-state overlay sections (none, two ticks, unavailable, stale) built from raw backend results through the legacy transform; Go tests prove records, cells, sorts, filters and that incremental application equals a rebuild for every pair of states, and a property test interleaves object churn with ticks of every size.
View engine host ladder (`lib/views/viewClientFactory.ts`, setting `performance.viewWorker` auto|off, default auto): the engine for each view runs at level W (a Web Worker built from an inline Blob fetches, parses and applies deltas itself; the main thread keeps a mirror of only the rows near the requested range and applies worker messages once per animation frame), M (worker applies, the main thread fetches and transfers ArrayBuffers) or I (inline, the plain view client). The level comes from the bulk probe caps (`workerBlob && workerFetch` gives W, `workerBlob` only gives M, anything else I) and is fixed when a view opens. Wails bindings and events do not exist inside a worker, so control calls (open, update, facet, selection) are rpc messages the main thread answers, and the single `view-updates` / `ws:reconnected` listener forwards notices as messages. Watchdog: a worker that never acks init within 2s goes straight to I; error, messageerror, 3 missed 2s heartbeats or repeated worker fetch failures step one level down (W to M, M to I) and every view re-opens from its retained spec, range, selection and activity on the new level. Large `Get*Yaml` / history / log page bodies parse in the worker through `fetchBulkInWorker` when the session is at level W. Refs lists above 10000 lines return as a transferred index buffer plus a string table. The PerformancePanel Transport tab shows the level, open views, notice rate, worker decode time, resident rows and watchdog events.
Custom resources (`pkg/rows/spec_crd.go`): a view of type `crd:group/version/resource` has no registered spec. `ViewManager.Open` resolves the CRD (`Config.CRD`, backed by `k8s.Client.ResolveCRDView`: scope plus the printer columns of the requested version) and builds a spec with `GenericCRDSpec`. Columns are name, namespace (namespaced resources), one per printer column and age; keys use the slug rule of the legacy list (lower case, whitespace to underscore, name/age/namespace and the creationTimestamp column skipped). Kinds follow the printer type (integer and number are numeric columns, boolean selects, date is not filterable), priority above zero is hidden by default, and search fields are name, namespace, uid, labels, annotations plus one per printer column. The schema travels in `ViewOpenResult.schema` (same shape as an entry of the generated file, plus `label` and `hidden` per column) and the frontend reads it from the view snapshot (`schemaFromOpenResult`, `resolveSchema`). The store fetch mode follows the columns: no printer columns use metadata, printer columns use the server Table (cells matched by column name), and a date column forces full objects because the server prints dates as ages. Whenever full objects are used the shard keeps only metadata plus the top-level fields the JSONPaths read (`Hint.Prune`, `pruneSource`); a pruned shard never answers `Manager.Lookup`, so detail views fetch the object live. A resource the CRD lookup cannot find still opens with the base columns and the failing shard reports through `ViewState.error`. `CustomResourceList` renders from the view when `performance.viewStore` is on; its legacy branch (browser-side JSONPath, anchored subscribe) is unchanged.
Warm readers of the store (`pkg/store/reader.go`, `pkg/k8s/storeread.go`): code that is not a UI list reads shards that a view or a pin already started, and never starts one. `k8s.StoreList[T](k8s.WithStoreRead(ctx), client, contextName, type, namespace)` answers from a synced watching full-object shard (a cluster-wide shard serves a namespace, never the reverse) and reports a miss otherwise so the caller lists live. Consumers: dependency graph list getters (`deps_cache.go`), the issue detector resource cache (`fetchWarm`, secrets stay live), the read-only list tools in `pkg/tools` (used by the MCP server), namespace resource counts (`StoreCounter`, any shard mode) and the metrics `PodNodeSource` (the metrics calls keep already-open cluster-wide pod and node shards pinned for a short time while the metrics UI polls, through `Manager.PinExisting`, and never start one; with no warm shard they list live). UI lists never set `WithStoreRead`.
Opt-in dedupe for the legacy watch path (`app_eventdedupe.go`, setting `performance.legacyEventDedupe`, default off): a MODIFIED event whose object equals the last emitted one apart from `resourceVersion` is dropped, while the tracked resumable version still advances. Objects that change on every update are unaffected.
Kill switches and settings: env `KUBIKLES_VIEWS=0` (views off), `KUBIKLES_DISABLE_BULK=1` (bulk routes off), `KUBIKLES_DISABLE_TABLE=1` (store never uses Table or metadata sources), `KUBIKLES_STORE_MAX_BYTES` (store byte budget, 0 unlimited), `KUBIKLES_ALLOWED_HOSTS` (server-mode Host check). Settings: `performance.viewStore`, `performance.viewWorker`, `performance.bulkTransport`, `performance.legacyEventDedupe`. The PerformancePanel Store tab lists shards (phase, mode and requested mode, entries, estimated bytes, leases, degraded and lingering flags) and open views (rows, window size, version, notices and bytes per minute).
Capability notes by platform (levels observed): Chrome via server mode, 2026-11-23: probe passed, auto selected W, `viewWorker=off` selected I with identical rows. macOS WKWebView desktop and Windows WebView2: not observed yet; run the Transport tab probe on each and record the level `auto` picks here (a failing worker fetch there means M or I by default with no further work).

### 10. Server-Mode Fan-Out (`pkg/server/`)
`EmitEvent` encodes each event once and enqueues it on every client's bounded queue (`client.go`) without blocking; a client that falls too far behind is disconnected and re-lists on reconnect. Large `/api` responses are gzipped when the client sends `Accept-Encoding: gzip`. Static files are pre-gzipped: hashed assets are served `immutable`, `index.html` is served `no-cache`.

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

// IssueDetectorContext
const { scanning, result, rules, runScan, groupBy, setGroupBy } = useIssueDetector();
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
- `[Resource]List.tsx` - List view with ResourceList component
- `use[Resource]Actions.tsx` - Edit, delete, view handlers
- `[Resource]ActionsMenu.tsx` - Context menu (optional)

## Adding New Resource

1. **Backend** (`pkg/k8s/client.go`): Add List/Get/Update/Delete methods
2. **Expose** (`app_[domain].go`): Wrap client methods, add watcher (use existing domain file or create new)
3. **Generate**: `wails generate module`
4. **Types** (`types/k8s.ts`): Add K8s resource type definition if needed
5. **Hook** (`hooks/use[Resource].tsx`): Copy pattern from existing, add explicit types
6. **Feature** (`features/[category]/[resource]/`): List + Actions
7. **Register** (`utils/resourceRegistry.ts`): Add resource config
8. **Route** (`App.tsx`): Add case in renderContent
9. **Sidebar** (`Sidebar.tsx`): Add navigation item

## Critical Files Quick Reference

| Task | File(s) |
|------|---------|
| Add K8s operation | `pkg/k8s/client.go` + `app_[domain].go` |
| Add view/feature | `App.tsx` + `Sidebar.tsx` + `features/` |
| Add context state | Relevant `context/*.tsx` |
| Add shared component | `components/shared/` |
| Add K8s resource type | `types/k8s.ts` |
| Configure resource | `utils/resourceRegistry.ts` |
| Theme customization | `app_themes.go` + `ThemeContext.tsx` |
| Port forward logic | `app_portforward.go` + `hooks/usePortForwards.tsx` |
| Terminal behavior | `app_terminal.go` + `pkg/terminal/` + `Terminal.tsx` |
| Log streaming | `app_logs.go` + `log-viewer/` |
| Helm operations | `app_helm.go` + `pkg/helm/` |
| Dependency graph | `pkg/k8s/dependencies.go` + `DependencyGraph.tsx` |
| AI integration | `app_ai.go` + `pkg/ai/` + `AIChatContext.tsx` |
| Issue detection | `app_issuedetector.go` + `pkg/issuedetector/` + `hooks/useIssueDetector.tsx` |
| MCP server | `pkg/mcp/server.go` |
| Watcher infrastructure | `app_watchermgr.go` + `app_watchers.go` |
| Performance metrics | `app_perfmetrics.go` |

## Build Commands

```bash
make dev              # Development with hot-reload
make build            # Current platform (with BUILD_TAGS=helm)
make build-lite       # Current platform without Helm
make build-release    # Optimized portable
make build-all        # All platforms (parallel)
make test             # Frontend tests
make generate         # Regenerate dispatch_gen.go
make analyze-size     # Binary size analysis
make profile          # Collect PGO profile
make build-pgo        # Build with PGO
```

## Build System

### Build Tags
- `helm` (default): Includes full Helm support. Controlled by `BUILD_TAGS` Makefile variable.
- `!helm` (lite): Stubs return `ErrHelmNotAvailable`. Build with `make build-lite`.
- Pattern follows `debugcluster` / `debugcluster_stub` convention.

### Generated Dispatcher
`dispatch_gen.go` maps method names to App methods (replaces reflect.MethodByName for DCE). Regenerate after adding/changing App methods: `make generate` or `go generate ./...`.

### Pre-compressed Assets
Frontend dist is gzip'd at build time (`scripts/compress-dist.sh`). Desktop mode decompresses via Wails middleware (`pkg/compressedassets`). Server mode serves `.gz` directly.

## Performance Notes

- Event coalescing: frame-window batches, adaptive backoff, ordered async delivery, slow cadence while hidden
- Reference-counted watchers
- List/watch handoff by resourceVersion avoids replaying the whole list as ADDED events
- Protobuf transport to the API server (JSON fallback via env var), managedFields stripped
- Paged list streaming (`list-page` events) for first paint before the full list returns
- Metrics lists shared through single-flight short-TTL caches; Prometheus history queries run concurrently with a bound
- ResourceList: pure sort/filter pipeline, memoised rows, store-backed metric cells, stable column definitions
- Bounded log store with frame-throttled snapshots; history paging without full log download
- Frontend build: flow, xterm and monaco are lazy chunks; bundled fonts, optional fonts loaded on demand
- Request-scoped caching in dependency graphs
- HTTP/1.1 option for parallelism
- Sequence tracking prevents stale updates
- PGO available (5-15% improvement)
- Runtime tuning (`runtime_tuning.go`): desktop and headless builds pick GC percent and a soft memory limit differently; GOGC/GOMEMLIMIT from the environment always win
- Performance dashboard reads memory stats via runtime/metrics, avoiding stop-the-world `ReadMemStats`
- Windowed views (opt-in): the shared store keeps one list+watch shard per context/type/namespace, Go views sort, filter and window rows, and the WebView holds only the visible window (see Key Systems 9c)
- A watch update that changes no projected cell produces no row and no notice (per-row sequence numbers with `have` give row-level deltas)
- Same-origin GET bulk transport with NDJSON streaming and a worker host ladder (W, M, I, legacy) chosen by an in-app probe; every level falls back to the binding per call
- Metrics polling pins cluster-wide pod and node shards for a short TTL so the next poll reads memory instead of listing again; dependency graph, issue detector and MCP list tools read warm shards and never start one
- Store byte budget defaults to a share of the container memory limit (idle shards are evicted first, leased shards never)

## Supported Resources

**Workloads**: Pods, Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs, CronJobs
**Cluster**: Nodes, Namespaces, Events, Metrics, PriorityClasses, Webhooks
**Config**: ConfigMaps, Secrets, HPAs, PDBs, ResourceQuotas, Leases, LimitRanges
**Storage**: PVs, PVCs, StorageClasses, CSI Drivers, CSI Nodes
**Network**: Services, Ingresses, NetworkPolicies, Endpoints, EndpointSlices, IngressClasses
**Access Control**: Roles, ClusterRoles, RoleBindings, ClusterRoleBindings, ServiceAccounts
**Custom**: CRDs and custom resource instances
**Helm**: Releases and repositories
