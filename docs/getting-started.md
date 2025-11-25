# Getting Started

## Prerequisites

- **Go** 1.24+ ([download](https://golang.org/dl/))
- **Node.js** 18+ ([download](https://nodejs.org/))
- **Wails CLI** v2

Install Wails:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Verify installation:
```bash
wails version
```

## Installation

1. **Clone the repository**
   ```bash
   git clone <repo-url>
   cd kubikles
   ```

2. **Install frontend dependencies**
   ```bash
   cd frontend
   npm install
   cd ..
   ```

## Development

Run in development mode with hot-reload:

```bash
make dev
```

Or using Wails directly:
```bash
wails dev
```

This will:
- Start the Go backend
- Start the Vite dev server with hot-reload
- Open the desktop application window
- Auto-reload on file changes

## Production Build

Build the application:

```bash
make build
```

The compiled binary will be in `build/bin/`.

## Project Structure

```
kubikles/
├── main.go              # Application entry point
├── app.go               # Backend methods
├── pkg/k8s/             # Kubernetes client
├── frontend/            # React application
│   ├── src/
│   │   ├── features/    # Resource views (pods, deployments, etc.)
│   │   ├── components/  # Shared UI components
│   │   ├── hooks/       # Data fetching hooks
│   │   └── context/     # State management
│   └── wailsjs/         # Auto-generated Wails bindings
└── build/               # Build configuration
```

## Troubleshooting

**Wails command not found**
Add Go bin to your PATH:
```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

**Node modules missing**
Run `npm install` in the `frontend/` directory.

**Port conflicts**
Wails dev mode uses dynamic ports. Ensure no conflicting processes.

## Next Steps

- See [Architecture](architecture.md) for a technical overview
- See [AI Reference](ai/README.md) for codebase patterns
