package crashlog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

const (
	logFileName    = "kubikles.log"
	maxLogSize     = 5 * 1024 * 1024 // 5MB
	maxLogFiles    = 3
)

var (
	logFile   *os.File
	logWriter io.Writer
)

// Init initializes crash logging. Should be called at the start of main().
// Returns a cleanup function that should be deferred.
func Init() func() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get config dir for crash log: %v\n", err)
		return func() {}
	}

	logDir := filepath.Join(configDir, "kubikles")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create log dir: %v\n", err)
		return func() {}
	}

	logPath := filepath.Join(logDir, logFileName)

	// Rotate logs if needed
	rotateLogsIfNeeded(logPath)

	// Open log file in append mode
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open crash log file: %v\n", err)
		return func() {}
	}

	// Write to both file and stderr
	logWriter = io.MultiWriter(os.Stderr, logFile)

	// Write startup marker
	writeLog("=== Application starting ===")
	writeLog("Time: %s", time.Now().Format(time.RFC3339))
	writeLog("Log file: %s", logPath)

	return cleanup
}

// cleanup closes the log file
func cleanup() {
	if logFile != nil {
		writeLog("=== Application shutting down normally ===")
		logFile.Close()
		logFile = nil
	}
}

// LogPanic should be called with defer at the start of main() to catch panics
func LogPanic() {
	if r := recover(); r != nil {
		writeLog("=== PANIC RECOVERED ===")
		writeLog("Time: %s", time.Now().Format(time.RFC3339))
		writeLog("Panic: %v", r)
		writeLog("Stack trace:\n%s", debug.Stack())
		writeLog("=== END PANIC ===")

		// Ensure the log is flushed
		if logFile != nil {
			logFile.Sync()
			logFile.Close()
		}

		// Re-panic to allow the OS to handle it (creates crash dialog on some systems)
		panic(r)
	}
}

// Log writes a message to the crash log
func Log(format string, args ...interface{}) {
	writeLog(format, args...)
}

// LogError writes an error message to the crash log
func LogError(format string, args ...interface{}) {
	writeLog("ERROR: "+format, args...)
}

// LogFatal writes a fatal error message and exits
func LogFatal(format string, args ...interface{}) {
	writeLog("FATAL: "+format, args...)
	if logFile != nil {
		logFile.Sync()
		logFile.Close()
	}
	os.Exit(1)
}

// Go runs a function in a new goroutine with panic recovery.
// Use this instead of `go func()` to ensure panics are logged.
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				writeLog("=== GOROUTINE PANIC ===")
				writeLog("Goroutine: %s", name)
				writeLog("Time: %s", time.Now().Format(time.RFC3339))
				writeLog("Panic: %v", r)
				writeLog("Stack trace:\n%s", debug.Stack())
				writeLog("=== END GOROUTINE PANIC ===")

				// Ensure the log is flushed
				if logFile != nil {
					logFile.Sync()
				}
			}
		}()
		fn()
	}()
}

func writeLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s\n", timestamp, msg)

	if logWriter != nil {
		fmt.Fprint(logWriter, line)
	} else {
		fmt.Fprint(os.Stderr, line)
	}
}

func rotateLogsIfNeeded(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil {
		return // File doesn't exist yet
	}

	if info.Size() < maxLogSize {
		return // Not big enough to rotate
	}

	// Rotate existing logs
	for i := maxLogFiles - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", logPath, i)
		newPath := fmt.Sprintf("%s.%d", logPath, i+1)
		os.Rename(oldPath, newPath)
	}

	// Move current log to .1
	os.Rename(logPath, logPath+".1")

	// Delete oldest if it exists
	os.Remove(fmt.Sprintf("%s.%d", logPath, maxLogFiles))
}

// GetLogPath returns the path to the crash log file
func GetLogPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "kubikles", logFileName)
}
