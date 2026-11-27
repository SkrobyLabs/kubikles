# Adversary review: perf/optimisation-pass (working tree vs main)

Date: 2026-11-27. Scope: all uncommitted work on the branch (HEAD equals main). Reviewers ran
read-only against the working tree; worktree isolation was not used because the work is uncommitted.

Severity: Critical (security vuln / data loss / crash), Major (significant correctness / missing
safeguard), Minor (code quality / edge case), Nit (style / naming / docs).

Status column: OPEN, FIXED, WONTFIX (with reason), or REFUTED (by meta-review).

## Wave 1

### A. Auth and exposed surfaces

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| A1 | Major | Bulk allowlist `List` prefix admits `ListPodFiles`, which execs in the pod and may add an ephemeral debug container (cannot be removed). Frontend mirror sends it as GET; transport fallback can run it twice. `TestAllowlistExcludesMutations` checks prefixes only. | pkg/bulk/allowlist.go:48, app_filetransfer.go:32, pkg/k8s/fileops.go:64, frontend bulkMethods / App.ts:67 | new | OPEN |
| A2 | Major | Host check (DNS-rebinding defence) covers only `/_kubikles/`; `/api/call` and `/ws` have none, and `/api/` sends `ACAO: *`, so any web page can call `GetBulkInfo` and read the bulk token. Surrounding exposure pre-existing; the partial guard is new. | pkg/bulk/host.go:8, server_mode.go:28-33, pkg/server/server.go:81,309-311 | partial | OPEN |
| A3 | Minor | View notices are broadcast to every server-mode client with view ids; any client can read, close, replace or deactivate another client's views. `ClientToken` / `ClientID` are client supplied, so the per-client cap is bypassable. | pkg/rows/manager.go:240,276-283, viewnotifier.go:77 | new | OPEN |
| A4 | Minor | `SetBulkTransport("off")` does not revoke an issued token; process-wide settings (`SetWindowVisible`, `SetMetricsOverlayConfig`, `SetLegacyEventDedupe`) are settable by any server-mode client. | app_bulk.go:93, pkg/bulk/guards.go:44, app.go:277 | new | OPEN |
| A5 | Minor | Startup log line always prints `http://localhost:%d` regardless of `KUBIKLES_BIND_ADDR`. | pkg/server/server.go:130 | new | OPEN |
| A6 | Nit | e2e hook: no minimum token length, no `ReadHeaderTimeout`, token left in process env (inherited by child shells). | e2e_hook.go | new | OPEN |
| A7 | Nit | Desktop bulk handler has no Host check (`AllowHost` nil); matters under `wails dev`. | main_desktop.go:43 | new | OPEN |
| A8 | Nit | `WailsMiddleware` now returns 304; Wails on Windows may turn 304 into an error / WebView2 hang. | pkg/compressedassets/handler.go:215 | new | OPEN |

### B. Go concurrency and state

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| B1 | Major | View rebuild can undo a just-applied `UpdateView`: `deliver` (reset path) and the replan goroutine read `v.cfg` under `mu`, release it, then wait for `opMu`; an `Update` in between installs a new cfg which the rebuild then overwrites. | pkg/rows/view.go:459-463,538-545 | new | OPEN |
| B2 | Major | Two `Open` calls with the same viewId leak a view: lookup and insert are separate lock sections; the overwritten view is never reaped, leases and overlay demand stay held, `byToken` drifts. | pkg/rows/manager.go:275-290 | new | OPEN |
| B3 | Major | Overlay demand on/off callbacks run after `h.mu` is released and can arrive out of order: poller stopped while wanted, or running with nobody wanting it (keeps cluster-wide shards warm). | pkg/rows/overlay.go:136-141,178-182,199-205, overlay_poller.go:79-101 | new | OPEN |
| B4 | Major | `StopAllWatchers` now resets the store and closes every view; the frontend calls it on every switch to polling (including the automatic fallback) and on context change in polling mode. Views never watch `lease.Done()`, so a view that acquired just before `store.Reset` keeps a stopped shard. | app_watchers.go:108, K8sContext.tsx:412,426, app_store.go:124-129, pkg/store/manager.go:591-603 | new | OPEN |
| B5 | Major | A resourceVersion from the old cluster can anchor a watcher on the new one after a context switch (hook checks its own closure's context, not the backend's). | app_watchermgr.go:72-102, app_watchers.go:74, useResourceWatcher.tsx:172, pkg/k8s/watch.go:142 | new | OPEN |
| B6 | Minor | `OnDomReady` -> `stopAllLogStreams` can cancel a stream the new page already started. | main_desktop.go:54 | new | OPEN |
| B7 | Minor | `SwitchContext` clears the coalescer before stopping watchers; a late emit lands in the new generation. | app_context.go:62-67 | pre-existing order | OPEN |
| B8 | Minor | Server idle timer: a stale `notifyIdle` can wipe a newer timer and fire with no grace; `OnNoClients` runs after the lock, so a reconnecting client's new stream can be cancelled. | pkg/server/server.go:215-219,288-293 | new | OPEN |
| B9 | Minor | Overlay fetchers ignore `ctx`; `poller.Close()` waits out a full metrics request. | app_overlays.go:222-256 | new | OPEN |
| B10 | Minor | `rebuildLocked` stops after three rounds even when another reset is pending. | pkg/rows/view.go:213-220 | new | OPEN |
| B11 | Nit | `Subscription.Cancel` doc vs behaviour; `Shard.pins` written never read; `prometheusFor` can repopulate after `resetViews`. | pkg/store/subscribe.go:28-33,146-151, pkg/store/manager.go:582, app_overlays.go:116-119 | new | OPEN |

### C. Input validation and injection

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| C1 | Major | RE2 fast path skipped for any regex with flags: the RE2 source is built from the regexp2-escaped `src` instead of `srcRE2`, RE2 rejects `\u`, and the pattern silently falls back to backtracking regexp2 (100 ms timeout per row, no total budget, under `v.mu`). Column filters always use `i`. | pkg/rows/query/jsregex.go:84, columnfilter.go:189, view.go:419-475,612 | new | OPEN |
| C2 | Major | Desktop bulk routes have no panic recovery (the Wails binding dispatcher has); a panic in an allowlisted method kills the app. Two reachable panics: `View.Refs` offset+limit overflow; `GetPodLogsBefore` limit+1 overflow. | main_desktop.go:43, pkg/bulk/guards.go:22, handler.go:112, pkg/rows/view.go:805-808, routes.go:149, app_views.go:241, pkg/k8s/podlogs_history.go:175,286-292 | new | OPEN |
| C3 | Major | Same as A1 (`ListPodFiles` through the allowlist). | | | DUP of A1 |
| C4 | Minor | Table API paths built by concatenation from unvalidated namespaces; `ViewSpec.Namespaces` never validated (path injection towards the API-server proxy, frontend-controlled only). Also `store.ParseCRDType`. | pkg/k8s/table.go:67-84, source_table.go:42,89, planner.go:22, core.go:244, key.go:38 | new | OPEN |
| C5 | Minor | No size limits on view specs (namespaces, filters, search length, selection uids). | pkg/rows | new | OPEN |
| C6 | Minor | Bulk token is not a boundary in server mode (readable through `/api/*`); docs should not present it as one. | app_bulk.go:77, docs/server-mode.md | new docs | OPEN |
| C7 | Minor | `ViewSpec.ClientID` is client supplied but drives cleanup; `Open` replaces a view with the same id without an owner check. | pkg/rows/manager.go:220,276-280 | new | DUP of A3 |
| C8 | Nit | `ReportBulkProbe` logs the whole map untruncated; `PrinterColumnKey` can yield `__proto__` keys on a plain object; `toStringCell` has no size cap. | app_bulk.go:115, rowItem.ts:32, spec_crd.go:290 | new | OPEN |

### D. Architecture

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| D1 | Major | Parity and staleness guards never run: `make test` and CI are frontend-only plus `go vet`; `TestGeneratedSchemaIsCurrent` and the Go half of the golden parity suite gate nothing; no staleness check for `dispatch_gen.go`. | Makefile:308, .github build.yml:45,54, pkg/rows/routes_test.go:282 | pre-existing CI, new risk | OPEN |
| D2 | Major | `performance.viewStore` gates only the frontend: legacy metrics polls call `pinMetricsShards` and start cluster-wide pod and node LIST+WATCH shards with the flag off; the worker call fetcher is installed at module load. `docs/architecture.md:104` contradicts the code. | app_nodes.go:37,45, app_store.go:166-178, viewClientFactory.ts:567, bulk/transport.ts:315,386 | new | OPEN |
| D3 | Major | Semantics duplicated between legacy and view paths and parity locks in legacy gaps (jobs unsortable columns, secrets Size/keys missing in view, pod status in Go and TS, search fields in three places; schema-vs-legacy test covers four kinds). | JobList.tsx:63-83, spec_jobs.go:196-198, SecretList.tsx:73-84, spec_secrets.go:64-67, spec_pods.go:24, k8s-helpers.ts, test/golden/rowSchemas.test.ts:14-19 | new | OPEN |
| D4 | Minor | Root `app_views.go` / `app_overlays.go` hold domain logic; `pkg/rows` imports `pkg/bulk` for two helpers. | app_views.go:328-436, app_overlays.go:137-256, pkg/rows/routes.go:41,51 | new | OPEN |
| D5 | Minor | Frontend singletons have no production lifecycle: `availability.ts` caches for the session, `degraded` not keyed by context, `resetViewsInfo` has no callers. | availability.ts:11-15,32 | new | OPEN |
| D6 | Minor | `docs/server-mode.md:126` falsely claims `KUBECONFIG` and in-cluster fallback. | pkg/k8s/client.go:206-221 | pre-existing | OPEN |
| D7 | Minor | Stale guidance: "Adding New K8s Resource" checklist omits row spec, semantics, viewColumns, registry, golden files, `go generate`; Makefile describes `generate` as dispatch-only. | .claude/rules/kubikles-context.md:26, Makefile:77,342 | new | OPEN |
| D8 | Minor | `ResourceList.tsx` interleaves both modes; each list must remember `viewPinnedColumns`. | ResourceList.tsx | new | OPEN |
| D9 | Nit | README embeds tuning constants and a pilot-list enumeration that will go stale; `features/viewColumnTestSupport.ts` missing from the index; stale comments in hooks/resources/index.ts:70,119; e2e tag never vetted in CI; `KUBIKLES_VIEWS=0` naming; one diff mixes many concerns. | docs/ai/README.md:327,419,424 | new | OPEN |

### E. Frontend view engine

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| E1 | Major | A reused parked view shows the previous query's rows as current; with an auto-open navigation (`navigateWithSearch(..., true)`) the auto-open effect reads `rowAt(0)` of the stale query and opens the wrong resource's details. | useResourceView.tsx:101-102, ResourceList.tsx:585-596,887-901,1137-1153, DependencyGraph.tsx:573, AIMarkdown.tsx:100 | new | OPEN |
| E2 | Major | A list stays on "Loading…" forever with no error when a shard never syncs (non-403 list failure puts the shard in Backoff, view phase stays `loading`, `useResourceView` ignores `st.error`). Legacy maps any error to `incomplete`. | pkg/store/shard.go:580-590, pkg/rows/view.go:667,676, ResourceList.tsx:1433-1436, useResourceView.tsx:214-216, useResource.tsx:398 | new | OPEN |
| E3 | Minor | Inactive views come back active on the server after recovery; parked views get reopened after a WebSocket blip. | viewClient.ts:213-220,274-334, pkg/rows/manager.go:287 | new | OPEN |
| E4 | Minor | Reactivating a parked pod view fires a burst of late pod notifications. | manager.go:242, transitions.go:21, usePodNotifications.tsx:209-215 | new | OPEN |
| E5 | Minor | Reuse path has no hook-level tests (injected transports bypass parking). | useResourceView.test.ts, useResourceView.tsx:108-110 | new | OPEN |
| E6 | Nit | Shrink delay can be starved by a repeated identical shrink; equality compares raw vs normalised range; parked inline handles still fetch a window every 30 s; park key ignores the CRD descriptor; status counts include parked views; peek and take can return different handles; resize listeners outlive an unmount mid-drag. | viewClientProxy.ts:222-236, viewClient.ts:222-229, useResourceView.tsx:148 | new | OPEN |

## Wave 2 and 3

### F. Error handling and resilience

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| F1 | Major | Store shards relist with no delay when a watch is rejected as stale (open error or in-stream 410); a successful list resets `failures`, so a persistent rejection becomes continuous full LISTs. `backoffDelay` has no jitter. A test asserts zero waits. | pkg/store/shard.go:393-399,522-526,548-549, reflector_test.go:171-185 | new | OPEN |
| F2 | Major | Legacy path: a rejected watch anchor silently replays without an RV and emits no `reconnecting`, so no reconcile runs; objects deleted between list and watch stay on screen. | app_watchers.go:264-268,351-365, app_watchanchor_test.go:77-121 | new | OPEN |
| F3 | Major | Degradation latches last the session with no production reset: `permanent` after ~6 s of failed opens marks the kind degraded (not keyed by context); failed `ensureViewsInfo` cached; bulk `caps.degraded` never clears by itself; worker ceiling never reset. | viewClient.ts:18-20,336-358, availability.ts:39, bulk/transport.ts:335-345, viewClientFactory.ts:262 | new | OPEN |
| F4 | Major | Automatic switch to polling trips on ordinary connectivity loss: failed watch-open errors contain `watch=true`, matching `/watch/` in `isStreamTransportError`; store shard errors feed the same `watcher-error` sink; legacy pods watcher and pinned pods shard share a key so both count. Only the user can switch back. | streamingCompatibility.ts:14, app_store.go:63-70, K8sContext.tsx:840-917 | logic pre-existing, amplified | OPEN |
| F5 | Minor | A failed first load no longer recovers by itself (watcher subscription now waits for a successful list). | useResource.tsx:316,389 | new | OPEN |
| F6 | Minor | Overlay fetchers ignore ctx (dup of B9); absent Prometheus is not cached so detection reruns up to 4 times per interval. | app_overlays.go:109-122,222-246, overlay_poller.go:178 | new | OPEN |
| F7 | Minor | A panicking reflector leaves a dead shard registered (`stopped` never closed, still in `m.shards`). | pkg/store/shard.go:432-440 | new | OPEN |
| F8 | Minor | `fallbackToFull` build failure loops with no wait. | pkg/store/shard.go:470-472,596-599 | new | OPEN |
| F9 | Minor | Log stream restart on reconnect: tail 200 so a longer gap loses lines silently; a failed restart is console-only and leaves a dead stream with no banner. | useLogStream.ts:515-517,532-534, app_logs.go:367 | new | OPEN |
| F10 | Minor | Bulk fallback reruns a method the server already ran; worker `fetch.call` is never aborted. | bulk/transport.ts:385-395,441-457 | new | OPEN |
| F11 | Nit | Watch ERROR event that is neither 410 nor 406 backs off without surfacing; refs NDJSON `filtered` header discarded. | shard.go:553-558, views/transport.ts:131 | new | OPEN |

### G. Data integrity and action targeting

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| G1 | Major | `getObject` can return a different object than the uid asked for: `Shard.find` falls back to name on a uid miss; the live path ignores `ref.UID`; `useWithFullObject` does not check the returned uid. A deleted-and-recreated same-name object (StatefulSet pod) is used for bulk Evict, Cordon, Scale and details. Delete/evict are name-based with no UID precondition in both paths. | pkg/store/shard.go:949-958, app_views.go:363-386, hooks/useViewObject.ts:13-23, PodList.tsx:156, NodeList.tsx:114, DeploymentList.tsx:74 | new (name-based delete pre-existing) | OPEN |
| G2 | Major | Filters on columns the view schema does not know are silently dropped while the chip stays visible (secrets `size`; saved views restore every filter). Select-all then Delete would act on unfiltered rows. | ResourceList.tsx:410-421,1126,1388, lib/views/viewColumns.ts:102, pkg/rows/columnfilter.go:171 | new | OPEN |
| G3 | Minor | Selection lags filter changes: `SelectionStale` truncated to 64 uids; `selectAllRefs` does not wait for an in-flight patch; bulk handlers never revalidate refs. | view.go:949-954, viewClient.ts:780, ResourceList.tsx:990-1009 | new | OPEN |
| G4 | Minor | Shift-click fails silently on busy lists ("view changed" swallowed). | view.go:407-416, ResourceList.tsx:981 | new | OPEN |
| G5 | Minor | Clicking a skeleton row calls `onRowClick(undefined)` and throws. | ResourceListRows.tsx:46-62, usePodActions.tsx:205 | new | OPEN |
| G6 | Minor | Reopen silently clears the selection on a cold shard. | viewClient.ts:322 | new | OPEN |
| G7 | Minor | Table rows without embedded metadata merge across namespaces on a cluster-wide shard. | source_table.go:69-78, entry.go:28-32 | new | OPEN |
| G8 | Minor | `ageTimestampFor` attaches `creationTimestamp` to a legacy `age` cell that shows another time (Event Age uses `firstTimestamp`); future timestamps tick as "-Ns". | LiveAge.tsx:52, EventList.tsx:124 | new | OPEN |
| G9 | Minor | Custom resource lists sort typed in Go but as strings in legacy; no golden test. | CustomResourceList.tsx:194-197, spec_crd.go:219-226 | new | OPEN |
| G10 | Nit | Header checkbox "all" is count-based; golden tests share the extracted `semantics.ts` with legacy, so an extraction mistake passes on both sides. | ResourceList.tsx:942 | new | OPEN |

### H. API contract (Go and TypeScript)

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| H1 | Major | Performance > Store tab throws when the store has no shards: Go sends `"shards": null`, TS calls `.length` / `.map`. Normal state with `viewStore` off. | pkg/store/manager.go:711,728-735, app_store.go:150-156, storeViewsStats.ts:24, StoreViewsSection.tsx:39,48,59 | new | OPEN |
| H2 | Minor | Jobs "Duration" always `-` in view mode unless "Start Time" is visible (render reads `cells.startTime`, not projected). | jobs/viewColumns.tsx:12,70, spec_jobs.go:148, ResourceList.tsx:1129-1135 | new | OPEN |
| H3 | Minor | "Replicas" cannot be shown on Deployments / StatefulSets in view mode: it is both pinned and a presented column, and pinned keys are dropped from the picker. | deployments/viewColumns.tsx:46, statefulsets/viewColumns.tsx:44, ResourceList.tsx:246-250 | new (from the picker fix) | OPEN |
| H4 | Minor | Notices update the phase but not the error; an error can be missing or stale for up to 30 s. | pkg/rows/types.go:152-171, view.go:516, viewClient.ts:14,566, useResourceView.tsx:205 | new | OPEN |
| H5 | Minor | `GetListOutcome` keyed by a client-chosen request id with a per-page counter; two browsers in server mode can collide and read each other's outcome and resourceVersion. | useResource.tsx:73-76,380-388, listrequests.go:176-198 | new | OPEN |
| H6 | Nit | `models.ts` / `App.d.ts` hand-maintained and stale; `"pods"` overlay name is a bare string; effective columns after `UpdateView` never returned; error text differs by transport; `ws:reconnected` fires before the new client id; stale comment on `'replaced'`. | frontend/wailsjs, viewClient.ts:552,649, runtime.ts:58-79 | new | OPEN |

### I. Frontend robustness

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| I1 | Major | Live log cap grows without limit while sitting at the top of a followed stream: every `startReached` raises the cap by 50k and nothing lowers it. | useLogStream.ts:330-331, logBuffer.ts:78-80, LogViewer.tsx:590-593 | new | OPEN |
| I2 | Major | Filter-only log search rebuilds the whole filtered row array on every published frame. | useLogSearch.ts:161-184 | new | OPEN |
| I3 | Major | ArrowUp cannot release the follow pin on a busy stream (40 px per press is within the 48 px slack), which used to work; every key counts as scrolling input. | followPin.ts:12,61, LogViewer.tsx:805,835-842 | new | OPEN |
| I4 | Major | Server mode: a websocket reconnect wipes log tabs including paused / all-loaded ones (`triggerRefresh` -> `fetchLogs`); the stream restart on reconnect is redundant and `startStreaming` can leave an orphan stream when calls overlap. | K8sContext.tsx:996-1000, LogViewer.tsx:314-318, useLogStream.ts:497-518,530-534 | new | OPEN |
| I5 | Minor | Horizontal scrolling releases the pin; the "held" flag can stick after Cmd-Tab mid-drag; returning to a tab ignores the pin. | LogViewer.tsx:808-818,850 | new | OPEN |
| I6 | Minor | No memory budget by size: `LogLine` WeakMaps keep normalised text and HTML for every rendered line. | LogLine.tsx:65-80 | new | OPEN |
| I7 | Minor | LiveAge animates every character once on the first tick after mount or scroll (class added to existing spans). | LiveAge.tsx:34 | new | OPEN |
| I8 | Minor | LiveAge accessibility and copy: `aria-label` on a generic span with `aria-hidden` children; U+00A0 in copied text. | LiveAge.tsx:36-37 | new | OPEN |
| I9 | Minor | Seconds show where nothing ticks: details panels freeze at "2m 13s"; legacy non-`age` time columns jump by 5 s. | formatting.ts:44, LiveAge.tsx:52 | new | OPEN |
| I10 | Minor | Facet refresh is a trailing debounce that never fires on a busy cluster. | ResourceList.tsx:1107-1111 | new | OPEN |
| I11 | Minor | Collapsed column widths saved by the old resize bug are never repaired. | ResourceList.tsx:540-542 | pre-existing data | OPEN |
| I12 | Nit | View-mode widths only widen; PerformancePanel interval has no in-flight guard; `ViewWorkerSection` ticks when hidden; one timer per LiveAge cell; `ageTimestampFor` parses dates per cell per render; `tabular-nums` only under 10 minutes. | ResourceList.tsx:505-508, PerformancePanel.tsx:541, TransportSection.tsx:32, ResourceListRows.tsx:126 | new | OPEN |

### J. Test coverage and quality

| ID | Sev | Finding | Where | New? | Status |
|----|-----|---------|-------|------|--------|
| J1 | Major | Bulk actions in view mode have no handler tests and the fakes can never fail; `getObjects` is all-or-nothing (one deleted pod fails the whole evict). | NodeList.tsx:108-126, PodList.tsx:150-166, lib/views/testSupport.ts:148-154, workerTestSupport.ts:73-77, lib/views/transport.ts:167-180, app_views.go:281-295 | new | OPEN |
| J2 | Major | Log stream lifecycle untested (`ws:reconnected`, LogViewer pin wiring, listener registration, `OnDomReady`). | useLogStream.ts:530-534, LogViewer.tsx:582-605,799-842, app.go:240, main_desktop.go:54 | new | OPEN |
| J3 | Major | View fakes behave better than the real system (filtered always equals total, search filters nothing, no deletes, open never slow, refs never 409, notices in order). | lib/views/testSupport.ts, workerTestSupport.ts:66-72 | new | OPEN |
| J4 | Major | `viewPending` never exercised; the loading/empty test cannot tell the two apart. | ResourceList.viewMode.test.tsx:107-108,314-321 | new | OPEN |
| J5 | Minor | `formatWhen` tests depend on the machine's locale (fail under fi_FI). | formatting.test.js:121-145 | new | OPEN |
| J6 | Minor | Golden parity checks can pass vacuously (collator test asserts only a type; non-English locale skips the drift check). | test/golden/generateGolden.test.ts:826-841 | new | OPEN |
| J7 | Minor | Untested: LiveAge / `ageTimestampFor`, PerformancePanel polling labels and hidden read, resize start, `e2e_hook.go` (never compiled in CI). | | new | OPEN |
| J8 | Minor | Wall-clock dependent tests (idle listener 150 ms, overlay polling sleeps, perf limits). | pkg/server/broadcast_test.go:183-197, app_overlays_test.go:110-128, pkg/rows/overlay_poller_test.go:50-100 | new | OPEN |
| J9 | Minor | Selection tests lock in count-based "all"; no test for the search-change to stale-notice window; swallowed errors untested. | ResourceList.tsx:942,963,981 | new | OPEN |
| J10 | Nit | Stale "DESIRED behaviour" comment, unused `pendingBehaviour`, weak assertions. | broadcast_test.go:17,142-145, BulkActionBar.test.tsx:12, viewWorker.test.ts:326 | new | OPEN |

## Finding summary (before meta-review)

| Area | Critical | Major | Minor | Nit rows |
|------|----------|-------|-------|----------|
| A Auth and surfaces | 0 | 2 | 3 | 3 |
| B Go concurrency | 0 | 5 | 5 | 1 |
| C Input validation | 0 | 2 (+1 dup) | 3 (+1 dup) | 1 |
| D Architecture | 0 | 3 | 5 | 1 |
| E Frontend view engine | 0 | 2 | 3 | 1 |
| F Resilience | 0 | 4 | 6 | 1 |
| G Data integrity | 0 | 2 | 7 | 1 |
| H API contract | 0 | 1 | 4 | 1 |
| I Frontend robustness | 0 | 4 | 7 | 1 |
| J Tests | 0 | 4 | 5 | 1 |
| Total | 0 | 29 | 48 | 12 |

## Cross-cutting themes

1. **Acting on the wrong thing.** G1 (object matched by name, not uid), G2 (dropped filter with a visible chip), E1 (stale rows of a reused view, wrong details auto-opened), G3 (selection lag). All sit between "what the user sees" and "what the action receives".
2. **The fallback to polling is too easy to trip and too costly when it trips.** F4 (trips on ordinary connectivity loss), B4 (the switch resets the store and closes every view), F3 (latches never reset), B5 / F2 (watch anchors across context or rejection).
3. **Unflagged blast radius.** D2 (store shards start with the flag off), H1 (Store tab crash in the default state), A1 (bulk transport is on by default and admits a mutating method), C2 (no panic recovery on the desktop bulk routes).
4. **Lifecycle of shared state.** B1, B2, B3 (view manager and overlay ordering), B8 / B6 (idle timer and page-load cleanup), F7 (dead shard), E3 (recovered views come back active), D5 (frontend singletons).
5. **Log viewer regressions.** I1 (unbounded cap), I2 (per-frame rebuild in filter mode), I3 (ArrowUp), I4 (reconnect wipes tabs), I5.
6. **Tests that cannot fail and guards that never run.** D1 (CI runs no Go tests), J1 to J4, J5, J6.

## Priority matrix (before meta-review)

Quick wins (under 30 minutes each): A1, C1, C2 (recover + clamps), H1, H2, H3, I3, I7, J5, B6, B8, A5, A6, D6, G5, I11, B2, F8.

Medium (1 to 4 hours each): E1, E2 + H4, G1, G2, B1, B3, B4, F1, F2, F4, I1, I4, D1, D2, F3, B5.

Larger (4+ hours): A2 / A3 (server-mode origin, host and per-client scoping), D3 (parity coverage for every migrated kind), J1 to J4 (fakes that can fail, handler tests), I2, G3.

## Meta-review

Pass 1 used three checkers that tried to disprove the findings against the source. No finding was
refuted outright. The Status columns in the tables above were not rewritten; this section is the
authoritative outcome and supersedes them.

### Corrections

- **Severity.** Findings that exist only with `performance.viewStore` on (opt-in) were lowered to
  Minor unless they can act on the wrong object: B3, C1, C2 (clamps), D3, F3, G2, I1, I2, I7, J1 to J4.
  Kept Major: D1, D2, F1, F2, H1 (default path), B1, B2 (view path), E1, E2, G1 (wrong target or stuck
  list), I3 (regression on the default path), F4 / B4 (polling fallback).
- **Wrong premises.** F1: client-go's reflector does wait between relists (about 0.8 s with jitter), so
  the no-wait relist was worse than the norm. E2: `useResourceView` did copy `state.error`; it was
  never rendered while the phase stayed `loading`. A1: the double run through the transport fallback
  is harmless because the helper is idempotent. I1 and I2 are not regressions (main had no cap and
  rescanned every line). G1: delete and evict are name-based in both paths, so the new exposure is
  the object shown and fetched, not a new wrong delete.
- **Wider than reported.** F2 also affected ordinary reconnects after a long gap, not only anchored
  watches: main emitted `reconnecting` on a stale-version open error and this branch had dropped it.
- **Duplicates.** C3 = A1. C7 and A4 fold into A3. C6 folds into A2. B9 = F6 (ctx half). F10 folds into
  A1. J1 + J3 are one test-fakes item. B6 + J2 share the page-load cleanup. H1 and D2 share a cause.
- **Priority matrix.** B6 was not a quick win (no correct backend-only fix); G1 and B4 were larger than
  Medium as first written.

### Gaps: areas of the diff no reviewer examined

`pkg/terminal/outputcoalescer.go` and `Terminal.tsx` (WebGL attach and detach on hidden tabs),
protobuf negotiation in `pkg/k8s/client.go`, the pagination changes in `pkg/k8s/listpages.go` /
`listrequests.go`, `pkg/compressedassets/handler.go` beyond the 304 note, the runtime memory tuning
files (`runtime_tuning.go`, `runtime_mem_*.go`: soft memory limit now a quarter of physical memory on
every build), font loading, `vite.config.js` chunking, `pkg/server/gzip.go`, `pkg/watchevents/`. These
have their own tests and pass, but received no adversarial review.

Server mode `/api/` and `/ws` are unauthenticated with any origin allowed. That predates this branch
and deserves its own change.

### Resolution

| ID | Outcome |
|----|---------|
| A1 | FIXED: deny set for `ListPodFiles` on both sides; a reviewed-list test fails when a new allowlisted method appears |
| A5 | FIXED: log line prints the real bind address |
| A6 | FIXED: e2e hook refuses short tokens, sets a header timeout, clears the token from the environment |
| B1 | FIXED: reset and replan rebuilds re-read the configuration once the operation lock is held |
| B2 | FIXED: replace of a view with the same id is one critical section |
| B3 | FIXED: poller demand is level-triggered from the hub |
| B4 | FIXED: `StopAllWatchers` no longer resets the store or closes views (the lease-after-reset window is OPEN) |
| B6 | FIXED differently: the backend hook was removed; the frontend remembers its stream ids across a reload and stops leftovers at page load (desktop only) |
| B8 | FIXED: idle timer carries a generation; a connect invalidates a pending callback |
| C1 | FIXED: flagged patterns stay on RE2 |
| C2 | FIXED: panic recovery on the bulk routes; overflow-safe `Refs`; log history limits clamped |
| D2 | FIXED: metrics polling pins only shards that already exist and never creates one |
| D6 | FIXED: `docs/server-mode.md` describes what the code does and states that the bulk token and Host check are not a security boundary |
| E1 | FIXED: a parked view is reused only when it holds the query a fresh open would send, and never for a list opened by a link carrying a search; hook-level tests added (E5) |
| E2 | FIXED: a failing first load reports incomplete, with the error and a retry |
| F1 | FIXED: stale relists back off after the first; jitter added |
| F2 | FIXED: `reconnecting` is emitted on a stale version so the frontend re-lists |
| F7, F8 | FIXED: a panicked shard is dropped and replaced; the fallback build failure waits |
| G1 | FIXED: a lookup that names a real uid matches that object only; a live fetch that returns a different uid is reported as not found. UID preconditions on delete and evict are OPEN |
| G2 | FIXED: filters on columns the view does not have are removed in view mode (adding secrets `size` to the schema is OPEN) |
| G5 | FIXED: clicks on placeholder rows are ignored |
| H1 | FIXED on both sides |
| H2, H3 | FIXED: carrier columns are separate from pinned ones; Replicas, Start Time and Completion Time stay available |
| I3, I5 | FIXED: upward keys release the pin at once; non-scrolling keys, command shortcuts and sideways scrolling are ignored; End resumes outright; tab return respects the pin |
| I4 | FIXED: reconnect leaves paused and fully loaded log tabs alone; the redundant restart is removed and overlapping starts are guarded |
| I7, I8 | FIXED: only changed digits animate; the age is plain text for copying and screen readers |
| J5 | FIXED: tooltip tests pass an explicit locale |

Open, to carry in the pull request description:

| ID | Why open |
|----|----------|
| A2, A3, A4, C6, H5 | Server mode scoping and origin checks: a design change, and the unauthenticated endpoints predate this branch |
| B5 | Old-cluster watch anchor after a context switch: needs a context argument on the subscribe binding |
| B7, B9, B10, B11 | Minor backend ordering and cleanup items |
| C4, C5, C8 | Validation and size limits on view specs (frontend-controlled input only) |
| D1 | CI runs no Go tests: one line in the workflow, but the wall-clock tests in J8 should be steadied first |
| D3, D4, D5, D7 to D9 | Parity coverage for every migrated kind, layering, stale guidance |
| E3, E4, E6 | Recovered views come back active; late pod notifications after reuse; small nits |
| F3 | Degradation latches last the session |
| F4 | The automatic switch to polling is over-eager. Mostly pre-existing; D2 and B4 remove this branch's amplification on the default path |
| F5, F6, F9 to F11 | Minor resilience items |
| G3, G4, G6 to G10 | Selection lag after filter changes, silent shift-click failure, others |
| H4, H6 | Error missing from notices; hand-maintained binding files |
| I1, I2, I6, I9 to I12 | Log cap ratchet, per-frame rebuild in filter-only search, per-line caches, minor list items. I11: column widths collapsed by the old resize bug stay saved; double-clicking the resize handle resets one |
| J1 to J4, J6 to J10 | Test fakes that cannot fail, handler tests, vacuous golden checks, wall-clock tests |

### Pass 2

A final pass reviewed the fixes themselves across 17 areas. Twelve were clean. It returned NOT
STABLE with five problems, all since addressed:

| Problem | Outcome |
|---------|---------|
| The stale-relist backoff (F1) did not engage when the Gone error arrived on the stream, the common case: the error event itself counted as healthy traffic and reset the counter. | FIXED: only non-error events reset it; a test with repeated in-stream Gone errors asserts the pause and fails without the fix |
| A shard already on full objects that is told the format is unsupported rebuilt the same source and looped with no wait. | FIXED: it waits the normal backoff |
| The page-load sweep of leftover log streams would stop the live page's own streams on a hot module reload (development only). | FIXED: runs once per page |
| Stream ids of streams that ended by themselves stayed in the ledger. | FIXED: forgotten on done and error |
| The "opened by a link" check for view reuse was read on the first render only and could miss a pending search consumed before the hook opened. | FIXED: latched across renders |

### Pass 3

A third pass reviewed those five fixes. Three were clean. It asked for two refinements to the relist
pacing, both made:

- The stale-relist counter was reset only by object events, so a quiet resource whose watch simply
  expires after minutes would have been paced more and more over the life of the shard. A watch that
  stayed open past the premature-close window now resets it too, with a test that fails without it.
- An "unsupported format" error event counted as healthy traffic and kept the retry delay at its
  first step. It no longer resets the failure count.

One accepted limitation from this pass: a list opened by a link disables view reuse for every list
mounted at that moment, for the life of those list instances. That costs speed, never correctness.

### Verification after all fixes

- `go vet -tags helm ./...` clean; `go test -tags helm -race ./...` passes.
- Frontend typecheck, lint and the full vitest suite pass.
- Scripted checks in the real desktop window (log follow, pause and resume, hidden window, stream
  counters) pass on a build of the final tree; log streams are released across a page reload.
- Keyboard follow behaviour (ArrowUp, PageUp, End, shortcuts) passes in headless WebKit and Chromium
  against a loopback server.
- Not exercised: destructive actions on a real cluster (the test cluster is read-only), Windows
  (WebView2), server-mode reconnect handling of log tabs.
