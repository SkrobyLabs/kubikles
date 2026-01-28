# Server / Headless Mode

Kubikles can run without a desktop environment, serving the full UI over HTTP with real-time updates via WebSocket. This is useful for:

- Headless servers, containers, and remote machines
- Running inside a Kubernetes cluster itself (in-cluster access via service account)
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

Deploy Kubikles as a pod inside the cluster itself. When running in-cluster, it automatically uses the pod's service account credentials - no kubeconfig mount needed.

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
      serviceAccountName: kubikles  # needs appropriate RBAC
      containers:
        - name: kubikles
          image: kubikles:latest
          ports:
            - containerPort: 8080
```

Pair with a `ClusterRole`/`ClusterRoleBinding` to control what the dashboard can access, and a `Service` or `Ingress` to expose it.

## Environment

Server mode uses the same kubeconfig resolution as the desktop app (`KUBECONFIG` env var or `~/.kube/config`). When running inside a cluster, it falls back to in-cluster service account credentials automatically.
