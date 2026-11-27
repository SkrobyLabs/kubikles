# Server / Headless Mode

Kubikles can run without a desktop environment, serving the full UI over HTTP with real-time updates via WebSocket. This is useful for:

- Headless servers, containers, and remote machines
- Running as a pod inside a Kubernetes cluster (with a kubeconfig file mounted into it)
- Platforms without Wails/WebKit support (e.g. FreeBSD, WSL)
- Accessing your cluster dashboard from any browser on the network

## Quick Start

**From a desktop build** (uses the same binary):

```bash
./kubikles -server -port 8080
```

**Headless build** (no GUI dependencies, smaller binary):

```bash
make build-headless
./build/bin/kubikles-headless -port 8080
```

Then open **http://localhost:8080** in your browser.

## Building

```bash
# Current platform
make build-headless

# Static Linux binaries (ideal for containers)
make build-headless-linux-amd64
make build-headless-linux-arm64

# Both Linux architectures
make build-headless-all
```

Headless builds use the `headless` Go build tag and disable CGO for fully static binaries.

## How It Works

Server mode replaces the Wails desktop bridge with:

- **HTTP API** (`/api/call`) - JSON-RPC style method dispatch. The frontend sends `{method, args}` and receives `{data}` or `{error}`.
- **WebSocket** (`/ws`) - Real-time events (resource updates, log streams, port forward status). Same event names and payloads as the desktop IPC.
- **Static file server** - The embedded frontend is served from `/`, with SPA routing support.

The frontend detects the runtime mode automatically and routes calls through the appropriate transport. No code changes are needed between desktop and server mode.

## Security

**Server mode has no authentication or authorization.** It exposes full cluster access (including exec, port-forward, secrets) to anyone who can reach the port.

Run it only:

- On `localhost` for personal use
- Behind a VPN or SSH tunnel for remote access
- With restricted network ingress (firewall rules, security groups)
- Behind a reverse proxy with authentication (e.g. OAuth2 Proxy, Authelia)

**Do not expose it directly to the internet.**

The `/api/` and `/ws` endpoints are unauthenticated and accept connections from any origin. The bulk token and the `Host` check on `/_kubikles/` only keep the browser's own page and simple cross-site requests out of the bulk routes; they are not a security boundary, because anyone who can reach the port can fetch the token through `/api/`. Bind server mode to a trusted interface (`KUBIKLES_BIND_ADDR=127.0.0.1`, or a private address) or put an authenticating proxy in front of it.

## Limitations

Some desktop-specific features behave differently in server mode:

| Feature | Desktop | Server Mode |
|---------|---------|-------------|
| File save/download dialogs | Native OS dialog | Browser download (limited) |
| Confirm dialogs | Native OS dialog | Not yet implemented |
| Open folder (themes, crash logs) | Opens Finder/Explorer | No-op |
| Drag-and-drop file upload | Full file path | Filename only (browser limitation) |
| Keyboard shortcuts (zoom) | Native menu | Not available |

## Container Deployment

Example `Dockerfile`:

```dockerfile
FROM alpine:latest
COPY kubikles-headless /usr/local/bin/kubikles
EXPOSE 8080
ENTRYPOINT ["kubikles", "-port", "8080"]
```

Mount your kubeconfig:

```bash
docker run -p 8080:8080 -v ~/.kube/config:/root/.kube/config kubikles
```

### In-Cluster Deployment

Deploy Kubikles as a pod inside the cluster itself. It does not use the pod's service account credentials: mount a kubeconfig at `~/.kube/config` of the container user (`/root/.kube/config` in the default image), for example from a Secret whose kubeconfig points at a service account token.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubikles
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubikles
  template:
    metadata:
      labels:
        app: kubikles
    spec:
      containers:
        - name: kubikles
          image: kubikles:latest
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: kubeconfig
              mountPath: /root/.kube/config
              subPath: config
              readOnly: true
      volumes:
        - name: kubeconfig
          secret:
            secretName: kubikles-kubeconfig  # a kubeconfig limited by RBAC
```

Give the identity in that kubeconfig a `ClusterRole`/`ClusterRoleBinding` that limits what the dashboard can access, and a `Service` or `Ingress` to expose it.

## Environment

Server mode reads the kubeconfig at `~/.kube/config` of the user running the process, plus any extra kubeconfig paths configured in the app settings. The `KUBECONFIG` environment variable is not read, and there is no in-cluster service account fallback: a pod that runs Kubikles must be given a kubeconfig file (for example from a mounted Secret at that path).

Other environment variables:

- `GOGC` / `GOMEMLIMIT`: headless builds choose their own GC percent and soft memory limit at startup; values set in the environment always win. In a container with a memory limit, set `GOMEMLIMIT` a little below it.
- `KUBIKLES_BIND_ADDR`: interface address to listen on (for example `127.0.0.1` for local-only use). Empty means all interfaces.
- `KUBIKLES_DISABLE_PROTOBUF=1`: use JSON instead of the Kubernetes protobuf wire format, for API gateways or proxies that only pass JSON.
- `KUBIKLES_DISABLE_BULK=1`: disable the bulk read transport (`/_kubikles/v1`); the browser then uses `/api/call` for everything.
- `KUBIKLES_VIEWS=0`: turn the windowed-list protocol off; every list then uses the legacy watch path.
- `KUBIKLES_DISABLE_TABLE=1`: never ask the API server for Table or metadata-only lists; the shared store holds full objects only. Use it behind a proxy or aggregated API that mishandles `as=Table`.
- `KUBIKLES_STORE_MAX_BYTES`: byte budget of the shared store's idle (unleased) shards; `0` means unlimited. Open views and pinned shards are never evicted.
- `KUBIKLES_ALLOWED_HOSTS`: comma-separated extra Host names accepted by the bulk routes (for example an ingress name, or `*.example.com`). `localhost`, IP literals and single-label names (a Kubernetes Service name) are always accepted. Other hosts only lose the bulk routes; the browser falls back to `/api/call`.

## Transport Behaviour

- **WebSocket fan-out**: each event is encoded once and queued per client without blocking. A client that cannot keep up (queue length or bytes exceeded, or a stalled socket) is disconnected, and the frontend re-lists on reconnect. One slow browser never delays the others.
- **Compression**: `/api` responses larger than about 1 KiB are gzipped when the request sends `Accept-Encoding: gzip`. Static assets are pre-gzipped at build time.
- **Caching**: hashed files under `/assets/` are served with `Cache-Control: public, max-age=31536000, immutable`; `index.html` is served with `no-cache` so a new deploy is picked up immediately.
- **Window visibility**: the backend only slows its update batching for a hidden window in desktop mode. In server mode the frontend does not report visibility, because one hidden tab would otherwise slow updates for every other connected viewer.

## Bulk Read Routes

Large read results can be fetched with plain GET requests under `/_kubikles/v1/`:

| Route | Purpose |
|-------|---------|
| `call/{Method}?args=<urlencoded JSON array>` | Allowlisted read methods (`List*`, `Get*Yaml`, history pages, pod/node metrics). Slice results are NDJSON (`application/x-ndjson`, one element per line); other results are `{"data":...}`; failures are `{"error":"..."}` with status 500. Arguments over 64 KB are refused with 414 (the client uses `/api/call` instead). |
| `probe/ping`, `probe/stream`, `probe/bench`, `probe/bytes` | Capability probe used by the frontend (`window.__kubiklesProbe()` runs it from the browser console). |

Rules for every `/_kubikles/` request: only GET and HEAD, `X-Kubikles: 1` plus `X-Kubikles-Token` (a per-process token the page obtains from `GetBulkInfo`), `Cache-Control: no-store`, no CORS headers, never a 304. Responses are gzipped when the client accepts it and are written in chunks of at most 64 KB. Each flushed chunk extends the write deadline, so a stream may outlast the server's 60 second `WriteTimeout`; a failed write ends the producer (the only cancellation signal).

## Windowed Views

When the browser setting `performance.viewStore` is on (Settings > Performance), supported lists, including custom resources, are sorted, filtered and windowed in the server instead of in the browser. The browser asks for a window of rows and receives only the rows near the viewport.

| Route | Purpose |
|-------|---------|
| `views/{id}/window?offset=&limit=&have=<version>` | One window as JSON: the uids in order plus the rows that changed after `have`. Unknown or expired views answer 410 and the client re-opens from its retained spec. |
| `views/{id}/refs?offset=&limit=` and `views/{id}/rows` | NDJSON streams of row references (select all, shift ranges) and of the whole projection. |
| `objects/{type}?ns=&name=` | One full object as JSON (`Accept: application/yaml` for YAML). Custom resources use `crd:group/version/resource` as the type. |

Control calls (open, update, close, facets, selection) go over the normal `/api/call` bindings. Updates arrive as small `view-updates` notices on the existing WebSocket: counts, phase and which views changed, never rows; the browser then fetches the window it shows.

Behaviour to know when running it for several people:

- The server has one current context for all browsers (as before). Browsers share the same store shard, so many people looking at the same list cost one watch against the API server; each browser has its own view (query, sort, window).
- A view is leased to the browser that opened it: it expires after about a minute without any call, and closes at once when the WebSocket disconnects. After a reconnect the browser re-fetches its windows. The number of views per browser and in total is capped; a browser over the cap falls back to the legacy lists.
- A hidden browser tab marks its views inactive, so its notices are held back and other tabs are unaffected.
- The shared store has a byte budget (`KUBIKLES_STORE_MAX_BYTES`). Only idle shards are evicted; a shard in use by a view is not.
- Custom resource lists use the server's Table output when the CRD defines printer columns, and fall back to a pruned object cache if the API server cannot produce Tables (or `KUBIKLES_DISABLE_TABLE=1`).
- Secret lists never copy secret values into the server's store: they use Table output, so the list shows the number of data keys; key names appear when the object is opened.

Reverse proxies: forward `/_kubikles/` and `/api` with the `Host` header intact and add the public name to `KUBIKLES_ALLOWED_HOSTS` (otherwise the `/_kubikles/` routes are refused and the browser uses `/api/call`). Do not buffer or cache `/_kubikles/` responses: NDJSON streams rely on flushed chunks and every response is `no-store`. The server compresses bodies itself, so let the proxy pass them through.
