# Kubikles™

Open-source Kubernetes desktop monitoring app by [SkrobyLabs](https://kubikles.app).

## Features

- Multi-cluster context switching
- Real-time resource monitoring with watchers
- Pod logs, terminal access, YAML editing
- Dependency graph visualization
- Support for Pods, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs, Services, ConfigMaps, Secrets, PVs, PVCs, StorageClasses, Nodes, Namespaces, Events

## Quick Start

### 1. Install Dependencies

```bash
make setup
```

This runs the cross-platform setup script that:
- Detects your OS and package manager
- Installs system dependencies (GTK3, WebKit2GTK on Linux)
- Installs Go and Node.js if missing
- Installs Wails CLI
- Installs frontend npm dependencies
- Runs `wails doctor` to verify the setup

**Supported platforms:** macOS, Linux (Arch, Debian/Ubuntu, Fedora, openSUSE)

### 2. Development

```bash
make dev
```

### 3. Build

```bash
# Build for current platform
make build

# Build optimized release
make build-release

# Build for Windows
make build-windows-amd64    # Intel/AMD (most PCs)
make build-windows-arm64    # ARM64 (Surface Pro X, etc.)

# Build for macOS
make build-mac              # Intel
make build-mac-arm          # Apple Silicon

# Build for Linux
make build-linux-amd64      # Intel/AMD
make build-linux-arm64      # ARM64
make build-appimage         # Portable AppImage

# Build all platforms
make build-all
```

Output binaries are in `build/bin/`.

**Linux users:** GTK3 and WebKit2GTK runtime libraries are required. See [Linux Support](docs/linux.md) for installation instructions.

See [Getting Started](docs/getting-started.md) for detailed setup instructions.

## Documentation

- [Developer Guide](docs/README.md) - Setup, architecture, development workflow
- [Server / Headless Mode](docs/server-mode.md) - Run without a desktop environment
- [Linux Support](docs/linux.md) - Runtime dependencies and AppImage packaging
- [AI Reference](docs/ai/README.md) - Codebase patterns for AI assistants

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go + client-go |
| Frontend | React + Vite + TailwindCSS |
| Desktop | Wails v2 |
| Editor | Monaco |
| Terminal | xterm.js |
| AI Pair | Claude (Anthropic) |

## Contributing

Contributions are welcome - see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Kubikles™ is a trademark of SkrobyLabs. The source code is licensed under the MIT License. See [LICENSE](LICENSE) and [TRADEMARKS.md](TRADEMARKS.md) for details.
