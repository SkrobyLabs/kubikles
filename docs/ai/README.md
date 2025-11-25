# Kubikles - AI Codebase Reference

This document provides efficient context for AI assistants working with the Kubikles codebase, eliminating the need to re-scan the project structure on each session.

## Project Overview

Kubikles is a lightweight Kubernetes desktop client built with **Go** (backend) and **React** (frontend), connected via **Wails** framework. It provides real-time cluster management with features like pod logs, YAML editing, terminal access, and dependency graph visualization.

---

## Directory Structure

```
kubikles/
├── main.go                    # Wails entry point
├── app.go                     # Main App struct - all backend methods exposed to frontend
├── go.mod                     # Go 1.24+
├── wails.json                 # Wails configuration
├── Makefile                   # make dev, make build
│
├── pkg/
│   ├── k8s/
│   │   ├── client.go          # Kubernetes client wrapper (List*, Get*, Update*, Delete*)
│   │   └── dependencies.go    # Dependency graph resolution
│   └── terminal/
│       └── terminal.go        # WebSocket terminal service
│
├── frontend/
│   ├── src/
│   │   ├── App.jsx            # Main layout + context providers
│   │   ├── context/
│   │   │   ├── K8sContext.jsx # Kubernetes state (contexts, namespaces)
│   │   │   └── UIContext.jsx  # UI state (views, tabs, modals)
│   │   ├── features/          # Feature-based organization
│   │   │   ├── cluster/       # nodes/, namespaces/, events/
│   │   │   ├── workloads/     # pods/, deployments/, statefulsets/, daemonsets/,
│   │   │   │                  # replicasets/, jobs/, cronjobs/
│   │   │   ├── config/        # configmaps/, secrets/
│   │   │   ├── network/       # services/
│   │   │   └── storage/       # pv/, pvc/, storageclass/
│   │   ├── components/
│   │   │   ├── layout/        # Sidebar.jsx, BottomPanel.jsx
│   │   │   └── shared/        # ResourceList.jsx, YamlEditor.jsx, LogViewer.jsx,
│   │   │                      # Terminal.jsx, DependencyGraph.jsx, ConfirmModal.jsx
│   │   ├── hooks/             # usePods.js, useDeployments.js, etc.
│   │   └── utils/
│   │       ├── resourceRegistry.js  # Central resource type definitions
│   │       ├── k8s-helpers.js       # Status helpers, pod filtering
│   │       ├── Logger.js            # Logging utility
│   │       └── formatting.js        # Date/time formatting
│   └── wailsjs/               # Auto-generated Wails bindings
│
└── build/bin/                 # Compiled binaries
```

---

## Key Patterns

### 1. Context Providers

**K8sContext** (`context/K8sContext.jsx`)
```javascript
// Provides:
const {
  contexts, currentContext, switchContext,
  namespaces, selectedNamespaces, setSelectedNamespaces,
  refreshContexts, refreshNamespaces, triggerRefresh
} = useK8s();
```

**UIContext** (`context/UIContext.jsx`)
```javascript
// Provides:
const {
  activeView, setActiveView,
  openTab, closeTab, bottomTabs, activeTabId,
  openModal, closeModal, modal,
  activeMenuId, setActiveMenuId,
  navigateWithSearch, consumePendingSearch
} = useUI();
```

### 2. Data Fetching Hooks

All resource hooks follow this pattern (`hooks/use[Resource].js`):

```javascript
export const useResource = (currentContext, selectedNamespaces, isVisible) => {
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!isVisible) return;  // Only fetch when visible

    // Fetch data
    const data = await ListResources(namespace);
    setItems(data);

    // Setup watcher
    StartResourceWatcher(namespace);

    // Listen for IPC events
    window.runtime.EventsOn("resource-event", (event) => {
      // Handle ADDED, MODIFIED, DELETED
    });
  }, [currentContext, selectedNamespaces, isVisible]);

  return { items, loading, error, setItems };
};
```

### 3. Action Hooks

Each feature has `use[Resource]Actions.jsx`:

```javascript
export const useResourceActions = () => {
  const { openTab, closeTab, openModal, closeModal } = useUI();
  const { currentContext } = useK8s();

  const handleEditYaml = (resource) => {
    openTab({
      id: `yaml-${resource.metadata.uid}`,
      title: `Edit: ${resource.metadata.name}`,
      content: <YamlEditor resourceType="..." namespace="..." resourceName="..." />
    });
  };

  const handleDelete = (resource) => {
    openModal({
      title: `Delete ${resource.metadata.name}?`,
      content: 'Are you sure?',
      confirmStyle: 'danger',
      onConfirm: async () => {
        await DeleteResource(namespace, name);
        closeModal();
      }
    });
  };

  return { handleEditYaml, handleDelete, ... };
};
```

### 4. Feature List Component

Each resource type follows (`features/[category]/[resource]/[Resource]List.jsx`):

```javascript
export default function ResourceList({ isVisible }) {
  const { currentContext, selectedNamespaces, namespaces } = useK8s();
  const { activeMenuId, setActiveMenuId } = useUI();
  const { items, loading } = useResource(currentContext, selectedNamespaces, isVisible);
  const { handleEditYaml, handleDelete } = useResourceActions();

  const columns = useMemo(() => [
    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
    { key: 'status', label: 'Status', render: (item) => <StatusBadge />, getValue: (item) => item.status },
    { key: 'actions', label: '...', render: (item) => <ActionsMenu ... />, isColumnSelector: true }
  ], []);

  return (
    <ResourceList
      title="Resources"
      columns={columns}
      data={items}
      isLoading={loading}
      namespaces={namespaces}
      currentNamespace={selectedNamespaces}
      onNamespaceChange={setSelectedNamespaces}
      multiSelectNamespaces={true}
      resourceType="resource"
    />
  );
}
```

### 5. Wails Communication

**Calling backend:**
```javascript
import { ListPods, DeletePod } from '../../wailsjs/go/main/App';
const pods = await ListPods(namespace);
await DeletePod(currentContext, namespace, name);
```

**Receiving events:**
```javascript
window.runtime.EventsOn("pod-event", (event) => {
  // event.type: "ADDED" | "MODIFIED" | "DELETED"
  // event.pod: the pod object
});
```

**Emitting from backend:**
```go
runtime.EventsEmit(a.ctx, "pod-event", PodEvent{Type: "ADDED", Pod: &pod})
```

---

## Adding a New Resource Type

### 1. Backend (`pkg/k8s/client.go`)
```go
func (c *Client) ListIngresses(namespace string) ([]netv1.Ingress, error) {
  cs, _ := c.getClientset()
  list, err := cs.NetworkingV1().Ingresses(namespace).List(context.TODO(), metav1.ListOptions{})
  return list.Items, err
}
```

### 2. Expose in `app.go`
```go
func (a *App) ListIngresses(namespace string) ([]netv1.Ingress, error) {
  return a.k8sClient.ListIngresses(namespace)
}
```

### 3. Generate bindings
```bash
wails generate module
```

### 4. Create hook (`hooks/useIngresses.js`)
Copy pattern from `usePods.js`, replace resource-specific parts.

### 5. Create feature folder
```
frontend/src/features/network/ingresses/
├── IngressList.jsx
├── useIngressActions.jsx
└── IngressActionsMenu.jsx (optional)
```

### 6. Register in `resourceRegistry.js`
```javascript
ingress: {
  kind: 'Ingress',
  plural: 'ingresses',
  namespaced: true,
  getYaml: (ns, name) => GetIngressYaml(ns, name),
  updateYaml: (ns, name, content) => UpdateIngressYaml(ns, name, content),
}
```

### 7. Add to App.jsx
```javascript
case 'ingresses': return <IngressList isVisible={true} />;
```

### 8. Add to Sidebar.jsx

---

## Critical Files

| File | Purpose |
|------|---------|
| `app.go` | All backend methods exposed to frontend |
| `pkg/k8s/client.go` | Kubernetes API operations |
| `frontend/src/App.jsx` | Main layout, view routing |
| `frontend/src/context/K8sContext.jsx` | Kubernetes state management |
| `frontend/src/context/UIContext.jsx` | UI state (tabs, modals, views) |
| `frontend/src/components/shared/ResourceList.jsx` | Universal data table |
| `frontend/src/utils/resourceRegistry.js` | Resource type definitions |
| `frontend/src/utils/k8s-helpers.js` | Status helpers, pod filtering |

---

## Common Operations

| Task | Location |
|------|----------|
| Add backend method | `app.go` + `pkg/k8s/client.go` |
| Add new view | `App.jsx` (renderContent) + `Sidebar.jsx` |
| Add tab content | Use `openTab()` from `useUI()` |
| Show confirmation | Use `openModal()` from `useUI()` |
| Get resource YAML | `resourceRegistry.js` → `getYaml(namespace, name)` |
| Navigate with search | `navigateWithSearch(view, searchTerm)` |
| Real-time updates | `StartResourceWatcher()` + `EventsOn("resource-event")` |

---

## Tech Stack Quick Reference

- **Backend**: Go 1.24+ with client-go (Kubernetes)
- **Frontend**: React 18 + Vite + TailwindCSS
- **Desktop**: Wails v2
- **Editor**: Monaco Editor
- **Terminal**: xterm.js
- **Graphs**: React Flow + dagre
