package main

// =============================================================================
// Imports
// =============================================================================

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"kubikles/pkg/ai"
	"kubikles/pkg/crashlog"
	"kubikles/pkg/debug"
	"kubikles/pkg/events"
	"kubikles/pkg/helm"
	"kubikles/pkg/issuedetector"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
	"kubikles/pkg/terminal"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// =============================================================================
// Types & Constants
// =============================================================================

// App struct
type App struct {
	ctx                   context.Context
	k8sClient             *k8s.Client
	k8sInitError          error // Stores K8s client initialization error for frontend display
	helmClient            *helm.Client
	terminalManager       *terminal.Manager
	aiManager             *ai.Manager
	watcherManager        *ResourceWatcherManager
	portForwardManager    *PortForwardManager
	ingressForwardManager *IngressForwardManager
	eventCoalescer        *EventCoalescer
	logCoalescer          *LogCoalescer
	themeManager          *ThemeManager
	scanEngine            *issuedetector.ScanEngine
	// Log streaming
	logStreams      map[string]context.CancelFunc
	logStreamsMutex sync.Mutex
	// Prometheus config cache (per context)
	prometheusConfigs     map[string]*k8s.PrometheusInfo
	prometheusConfigPath  string
	prometheusConfigMutex sync.RWMutex
	// Performance tracking
	perfMutex            sync.RWMutex
	maxGoroutines        int
	totalWatchersCreated int64
	totalWatchersCleaned int64
	// Event tracking per watcher key
	eventStatsMutex  sync.RWMutex
	eventStats       map[string]*WatcherEventStats
	eventWindowStart int64 // Unix ms when tracking started
	// Metrics request cancellation
	metricsRequestManager *MetricsRequestManager
	// List request cancellation
	listRequestManager *ListRequestManager
	// Connection test cancellation
	connTestMutex  sync.Mutex
	connTestCancel context.CancelFunc
	// Event emission (unified for desktop and server modes)
	emitter events.Emitter
}

// Watcher event types: see app_watchers.go (ResourceEvent, WatcherErrorEvent, WatcherStatusEvent)
// Performance metrics types: see app_perfmetrics.go (WatcherEventStats, PerformanceMetrics)
// Resource Watcher Manager: see app_watchermgr.go

// =============================================================================
// App Lifecycle
// =============================================================================

// NewApp creates a new App application struct
func NewApp() *App {
	client, err := k8s.NewClient()
	if err != nil {
		fmt.Printf("Error initializing K8s client: %v\n", err)
		// Continue - UI will show the error via GetK8sInitError()
	}

	// Setup prometheus config path
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "kubikles")
	os.MkdirAll(appDir, 0755)

	return &App{
		k8sClient:             client,
		k8sInitError:          err,
		helmClient:            initHelm(),
		terminalManager:       terminal.NewManager(),
		aiManager:             newAIManager(client),
		logStreams:            make(map[string]context.CancelFunc),
		prometheusConfigs:     make(map[string]*k8s.PrometheusInfo),
		prometheusConfigPath:  filepath.Join(appDir, "prometheus_config.json"),
		metricsRequestManager: NewMetricsRequestManager(),
		listRequestManager:    NewListRequestManager(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	crashlog.Log("App startup initiated")

	a.ctx = ctx
	// Set up Wails emitter for desktop mode
	a.emitter = events.NewWailsEmitter(ctx)
	// Initialize structured debug logger
	debug.Init(a.emitter)

	a.watcherManager = NewResourceWatcherManager(ctx, a)
	a.portForwardManager = NewPortForwardManager(a)
	a.ingressForwardManager = NewIngressForwardManager(a)
	a.eventCoalescer = NewEventCoalescer(a, 16*time.Millisecond) // 60fps frame batching
	a.logCoalescer = NewLogCoalescer(a, 16*time.Millisecond)     // 60fps log batching
	a.loadPrometheusConfigs()
	// Initialize theme manager
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "kubikles")
	a.themeManager = NewThemeManager(a, appDir)
	// Initialize issue detector
	rulesDir := filepath.Join(appDir, "rules")
	os.MkdirAll(rulesDir, 0755)
	a.scanEngine = issuedetector.NewScanEngine(rulesDir, func(p issuedetector.ScanProgress) {
		a.emitEvent("issuedetector:progress", p)
	})
	// Initialize event tracking
	a.eventStats = make(map[string]*WatcherEventStats)
	a.eventWindowStart = time.Now().UnixMilli()
	// Set event emitter on terminal manager
	if a.terminalManager != nil {
		a.terminalManager.SetContext(ctx)
		a.terminalManager.SetEmitter(a.emitter)
	}
	// Set context on AI manager for event emission
	if a.aiManager != nil {
		a.aiManager.SetContext(ctx)
	}

	// Log K8s client status
	if a.k8sInitError != nil {
		crashlog.LogError("K8s client initialization failed: %v", a.k8sInitError)
	} else {
		crashlog.Log("K8s client initialized successfully")
	}

	crashlog.Log("App startup complete")
}

// SetEmitter sets the event emitter (used for server mode)
func (a *App) SetEmitter(emitter events.Emitter) {
	a.emitter = emitter
}

// startupServerMode initializes the app for server mode (no Wails context).
// The emitter must be set via SetEmitter before calling this.
func (a *App) startupServerMode(ctx context.Context) {
	crashlog.Log("App startup initiated (server mode)")

	a.ctx = ctx
	// Initialize structured debug logger (emitter set via SetEmitter before this call)
	if a.emitter != nil {
		debug.Init(a.emitter)
	}
	a.watcherManager = NewResourceWatcherManager(ctx, a)
	a.portForwardManager = NewPortForwardManager(a)
	a.ingressForwardManager = NewIngressForwardManager(a)
	a.eventCoalescer = NewEventCoalescer(a, 16*time.Millisecond)
	a.logCoalescer = NewLogCoalescer(a, 16*time.Millisecond)
	a.loadPrometheusConfigs()

	// Initialize theme manager
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "kubikles")
	a.themeManager = NewThemeManager(a, appDir)
	// Initialize issue detector
	rulesDir := filepath.Join(appDir, "rules")
	os.MkdirAll(rulesDir, 0755)
	a.scanEngine = issuedetector.NewScanEngine(rulesDir, func(p issuedetector.ScanProgress) {
		a.emitEvent("issuedetector:progress", p)
	})

	// Initialize event tracking
	a.eventStats = make(map[string]*WatcherEventStats)
	a.eventWindowStart = time.Now().UnixMilli()

	// Set emitter on terminal manager
	if a.terminalManager != nil {
		a.terminalManager.SetContext(ctx)
		a.terminalManager.SetEmitter(a.emitter)
	}

	// Set emitter on AI manager (server mode uses custom emitter via app.emitter)
	if a.aiManager != nil {
		a.aiManager.SetEmitter(a.emitter, ctx)
	}

	if a.k8sInitError != nil {
		crashlog.LogError("K8s client initialization failed: %v", a.k8sInitError)
	} else {
		crashlog.Log("K8s client initialized successfully")
	}

	crashlog.Log("App startup complete (server mode)")
}

// getDisconnectListeners returns components that need to clean up when a WebSocket client disconnects.
// Used by server mode to register listeners for session cleanup.
func (a *App) getDisconnectListeners() []server.DisconnectListener {
	var listeners []server.DisconnectListener
	if a.aiManager != nil {
		listeners = append(listeners, a.aiManager)
	}
	if a.terminalManager != nil {
		listeners = append(listeners, a.terminalManager)
	}
	return listeners
}

// emitEvent sends an event to the frontend via the configured emitter.
func (a *App) emitEvent(name string, data ...interface{}) {
	if a.emitter != nil {
		a.emitter.Emit(name, data...)
	}
}

// ListProgress represents progress of a paginated list operation.
type ListProgress struct {
	ResourceType string `json:"resourceType"`
	Loaded       int    `json:"loaded"`
	Total        int    `json:"total"`
}

// listProgressCallback creates a progress callback that emits "list-progress" events.
func (a *App) listProgressCallback(resourceType string) func(loaded, total int) {
	return func(loaded, total int) {
		debug.LogK8s("list-progress", map[string]interface{}{"resource": resourceType, "loaded": loaded, "total": total})
		a.emitEvent("list-progress", ListProgress{
			ResourceType: resourceType,
			Loaded:       loaded,
			Total:        total,
		})
	}
}

// openBrowserURL opens a URL in the system browser (desktop mode only).
// In server mode, this is a no-op - the frontend handles URLs.
func (a *App) openBrowserURL(url string) {
	// Only works with WailsEmitter (desktop mode)
	if wailsEmitter, ok := a.emitter.(*events.WailsEmitter); ok && wailsEmitter != nil {
		runtime.BrowserOpenURL(a.ctx, url)
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	debug.LogWails("App shutdown initiated", nil)

	// Flush any pending coalesced events
	if a.eventCoalescer != nil {
		a.eventCoalescer.FlushNow()
	}

	// Clean up ingress forwarding (removes hosts file entries)
	if a.ingressForwardManager != nil {
		a.ingressForwardManager.Cleanup()
	}

	// Stop all port forwards
	if a.portForwardManager != nil {
		a.portForwardManager.StopAll()
	}

	// Stop all watchers
	if a.watcherManager != nil {
		a.watcherManager.StopAll()
	}

	// Close all terminal sessions
	if a.terminalManager != nil {
		a.terminalManager.CloseAllSessions()
	}

	// Close all AI sessions
	if a.aiManager != nil {
		a.aiManager.CloseAllSessions()
	}

	debug.LogWails("App shutdown complete", nil)
}

// AI: see app_ai.go
// Performance: see app_perfmetrics.go
// Configuration: see app_config.go
// Context: see app_context.go
// Themes: see app_themes.go
// Pods: see app_pods.go
// Namespaces: see app_namespaces.go
// Events: see app_events.go
// Services: see app_services.go
// Ingresses: see app_ingresses.go
// ConfigMaps: see app_configmaps.go
// Deployments: see app_deployments.go
// Debug: see app_debug.go
// Terminal: see app_terminal.go
