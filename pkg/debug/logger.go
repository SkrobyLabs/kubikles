package debug

import (
	"fmt"
	"sync"
	"time"

	"kubikles/pkg/events"
)

// Categories for debug logging (must match frontend DEBUG_CATEGORIES)
const (
	CategoryK8s         = "k8s"
	CategoryWatcher     = "watcher"
	CategoryHelm        = "helm"
	CategoryPortforward = "portforward"
	CategoryTerminal    = "terminal"
	CategoryAI          = "ai"
	CategoryConfig      = "config"
	CategoryUI          = "ui"
	CategoryWails       = "wails"
	CategoryPerformance = "performance"
)

// Logger provides debug logging that emits events to the frontend
type Logger struct {
	emitter events.Emitter
	enabled bool
	mu      sync.RWMutex
}

// Global logger instance
var globalLogger *Logger
var once sync.Once

// Init initializes the global debug logger with the event emitter
func Init(emitter events.Emitter) {
	once.Do(func() {
		globalLogger = &Logger{
			emitter: emitter,
			enabled: false,
		}
	})
	// Update emitter if called again
	if globalLogger != nil {
		globalLogger.mu.Lock()
		globalLogger.emitter = emitter
		globalLogger.mu.Unlock()
	}
}

// SetEnabled enables or disables debug logging
func SetEnabled(enabled bool) {
	if globalLogger == nil {
		return
	}
	globalLogger.mu.Lock()
	globalLogger.enabled = enabled
	globalLogger.mu.Unlock()
}

// IsEnabled returns whether debug logging is enabled
func IsEnabled() bool {
	if globalLogger == nil {
		return false
	}
	globalLogger.mu.RLock()
	defer globalLogger.mu.RUnlock()
	return globalLogger.enabled
}

// Log emits a debug log event to the frontend
func Log(category, message string, details map[string]interface{}) {
	if globalLogger == nil {
		return
	}

	globalLogger.mu.RLock()
	enabled := globalLogger.enabled
	emitter := globalLogger.emitter
	globalLogger.mu.RUnlock()

	if !enabled || emitter == nil {
		return
	}

	// Also print to stdout for development
	timestamp := time.Now().Format("15:04:05.000")
	if details != nil {
		fmt.Printf("DEBUG [%s] [%s] %s %v\n", timestamp, category, message, details)
	} else {
		fmt.Printf("DEBUG [%s] [%s] %s\n", timestamp, category, message)
	}

	emitter.Emit("debug:log", category, message, details)
}

// Convenience functions for each category

func LogK8s(message string, details map[string]interface{}) {
	Log(CategoryK8s, message, details)
}

func LogWatcher(message string, details map[string]interface{}) {
	Log(CategoryWatcher, message, details)
}

func LogHelm(message string, details map[string]interface{}) {
	Log(CategoryHelm, message, details)
}

func LogPortforward(message string, details map[string]interface{}) {
	Log(CategoryPortforward, message, details)
}

func LogTerminal(message string, details map[string]interface{}) {
	Log(CategoryTerminal, message, details)
}

func LogAI(message string, details map[string]interface{}) {
	Log(CategoryAI, message, details)
}

func LogConfig(message string, details map[string]interface{}) {
	Log(CategoryConfig, message, details)
}

func LogUI(message string, details map[string]interface{}) {
	Log(CategoryUI, message, details)
}

func LogWails(message string, details map[string]interface{}) {
	Log(CategoryWails, message, details)
}

func LogPerformance(message string, details map[string]interface{}) {
	Log(CategoryPerformance, message, details)
}
