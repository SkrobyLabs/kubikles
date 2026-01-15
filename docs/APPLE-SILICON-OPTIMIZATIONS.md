# Apple Silicon Optimizations

Performance optimizations targeting Apple Silicon (M1/M2/M3/M4) Macs.

## Implementation Checklist

### 1. XTerm.js WebGL Renderer - DONE
- [x] Install `@xterm/addon-webgl` package
- [x] Load WebGL addon in Terminal.jsx
- [x] Add fallback for WebGL unavailable
- [x] Handle context loss gracefully
- **Impact:** HIGH - 10x faster terminal rendering
- **Effort:** 30 min

### 2. CSS GPU Acceleration Hints - DONE
- [x] Add `will-change: transform` to virtualized scroll containers
- [x] Add `transform: translateZ(0)` for GPU layer promotion
- [x] Add `backface-visibility: hidden` where appropriate
- [x] Target: Virtuoso lists, React Flow, Terminal
- [x] Add `overscroll-behavior: contain` for trackpad
- **Impact:** MEDIUM - smoother 120Hz scrolling
- **Effort:** 15 min

### 3. React Flow Performance Flags - DONE
- [x] Add `elevateEdgesOnSelect={false}`
- [x] Add `panOnScroll={true}` for trackpad
- [x] Add `nodesDraggable={false}` (dagre handles layout)
- [x] Add `nodesConnectable={false}`
- [x] Add `elementsSelectable={false}`
- **Impact:** MEDIUM - faster dependency graph
- **Effort:** 15 min

### 4. CSS content-visibility - DONE
- [x] Add containment for Monaco editor
- [x] Add `contain: layout style paint` for detail panels
- [x] Add content-visibility for collapsed details elements
- **Impact:** LOW-MEDIUM
- **Effort:** 30 min

### 5. macOS Window Vibrancy (DEFERRED)
- [ ] Enable `WindowIsTranslucent` in Wails options
- [ ] Enable `WebviewIsTransparent`
- [ ] Add `backdrop-filter: blur()` to sidebar
- [ ] Adjust all theme background colors for alpha
- **Impact:** LOW (visual polish only)
- **Effort:** 1-2 hours
- **Note:** Deferred - requires theme color changes, purely cosmetic

### 6. Profile-Guided Optimization (PGO) - DONE
- [x] Create `make profile` target for collecting CPU profiles
- [x] Create `make build-pgo` for optimized builds
- [x] Create `make build-mac-arm-pgo` for Apple Silicon release
- [x] Add helper script `scripts/collect-pgo-profile.sh`
- **Impact:** MEDIUM - 5-15% faster Go code
- **Effort:** 2 hours

### 7. Apple Silicon Runtime Tuning - DONE
- [x] Add `runtime_darwin_arm64.go` with optimized settings
- [x] Set GOGC=150 (less frequent GC, safe with unified memory)
- [x] Set GOMEMLIMIT=2GB (prevent unnecessary GC pressure)
- [x] Ensure GOMAXPROCS uses all cores (P + E cores)
- **Impact:** LOW-MEDIUM - smoother performance, fewer GC pauses
- **Effort:** 15 min

## Technical Notes

### WebGL in xterm.js
The WebGL renderer uses Metal on macOS for GPU-accelerated text rendering.
Falls back gracefully to canvas if WebGL unavailable.

```javascript
import { WebglAddon } from 'xterm-addon-webgl';

// In terminal setup
try {
  const webglAddon = new WebglAddon();
  webglAddon.onContextLost(() => {
    webglAddon.dispose();
  });
  terminal.loadAddon(webglAddon);
} catch (e) {
  console.warn('WebGL addon failed to load, using canvas renderer');
}
```

### GPU Layer Promotion
CSS properties that trigger GPU compositing:
- `transform: translateZ(0)` or `translate3d(0,0,0)`
- `will-change: transform`
- `backface-visibility: hidden`

Use sparingly - too many layers can hurt performance.

### React Flow Optimization
Key props for performance:
- `elevateEdgesOnSelect={false}` - prevents edge re-renders
- `panOnScroll={true}` - native trackpad feel
- `minZoom/maxZoom` - prevents extreme zoom levels
- `nodesDraggable={false}` - if dragging not needed

### macOS Vibrancy
Requires both Wails and CSS configuration:
- Wails: `WindowIsTranslucent: true`
- CSS: `backdrop-filter: blur(20px)` on translucent elements
- Background colors need alpha: `rgba(30, 30, 30, 0.8)`

### Profile-Guided Optimization (PGO)
PGO uses runtime profiling data to optimize hot paths.

```bash
# Step 1: Collect profile (use app normally for 30-60s)
make profile

# Step 2: Build with optimization
make build-pgo              # Current platform
make build-mac-arm-pgo      # Apple Silicon release
```

Key points:
- Profiles are architecture-specific (arm64 profile works on ALL Apple Silicon)
- Profile from M1 works on M2, M3, M4, etc.
- Typical improvement: 5-15% faster execution
- Profile should represent typical usage patterns

### Apple Silicon Runtime Tuning
The `runtime_darwin_arm64.go` file applies these optimizations:

```go
// GOGC=150 - Less frequent GC (default is 100)
// Safe on Apple Silicon due to unified memory architecture
debug.SetGCPercent(150)

// GOMEMLIMIT=2GB - Soft memory limit
// Prevents unnecessary GC pressure on systems with plenty of RAM
debug.SetMemoryLimit(2 * 1024 * 1024 * 1024)
```

These settings can be overridden via environment variables:
```bash
GOGC=100 GOMEMLIMIT=1GiB ./Kubikles  # Use defaults
```
