package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// BoundaryMode selects the HTTP compatibility or hardened Accelerator boundary.
type BoundaryMode string

const (
	BoundaryModeCompatibility BoundaryMode = "compatibility"
	BoundaryModeAccelerator   BoundaryMode = "accelerator"
)

const AcceleratorMaxRequestBodyBytes int64 = 1 << 20

var (
	ErrInvalidBoundaryMode         = errors.New("invalid server boundary mode")
	ErrInvalidListenAddress        = errors.New("invalid server listen address")
	ErrProtectedRouteGuardRequired = errors.New("protected route guard required")
)

// ReadinessProvider reports whether the process is ready to receive protected work.
type ReadinessProvider interface {
	Ready(context.Context) error
}

// ReadinessFunc adapts a function to ReadinessProvider.
type ReadinessFunc func(context.Context) error

func (f ReadinessFunc) Ready(ctx context.Context) error { return f(ctx) }

// ProtectedRouteGuard wraps an Accelerator protected route.
type ProtectedRouteGuard func(http.Handler) http.Handler

// Options defines the server's fixed listener and HTTP boundary.
type Options struct {
	ListenAddress       string
	BoundaryMode        BoundaryMode
	ReadinessProvider   ReadinessProvider
	ProtectedRouteGuard ProtectedRouteGuard
}

// CompatibilityOptions preserves the ordinary server wildcard bind and HTTP surface.
func CompatibilityOptions(port int, readiness ReadinessProvider) Options {
	return Options{
		ListenAddress:     fmt.Sprintf(":%d", port),
		BoundaryMode:      BoundaryModeCompatibility,
		ReadinessProvider: readiness,
	}
}

// AcceleratorOptions selects the hardened loopback-only Accelerator boundary.
func AcceleratorOptions(port int, readiness ReadinessProvider, guard ProtectedRouteGuard) Options {
	return Options{
		ListenAddress:       fmt.Sprintf("127.0.0.1:%d", port),
		BoundaryMode:        BoundaryModeAccelerator,
		ReadinessProvider:   readiness,
		ProtectedRouteGuard: guard,
	}
}

// DenyProtectedRoutes keeps protected Accelerator routes unavailable until authentication is installed.
func DenyProtectedRoutes(_ http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = writeJSON(w, map[string]interface{}{"error": "unauthorized"})
	})
}

func validateOptions(options Options) error {
	switch options.BoundaryMode {
	case BoundaryModeCompatibility:
		if err := validateListenAddress(options.ListenAddress, false); err != nil {
			return err
		}
	case BoundaryModeAccelerator:
		if options.ProtectedRouteGuard == nil {
			return ErrProtectedRouteGuardRequired
		}
		if err := validateListenAddress(options.ListenAddress, true); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidBoundaryMode, options.BoundaryMode)
	}
	return nil
}

func validateListenAddress(address string, accelerator bool) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %q: %v", ErrInvalidListenAddress, address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || portText == "" || strings.IndexFunc(portText, func(r rune) bool { return r < '0' || r > '9' }) >= 0 || port < 0 || port > 65535 {
		return fmt.Errorf("%w: %q", ErrInvalidListenAddress, address)
	}
	if accelerator {
		if host != "127.0.0.1" {
			return fmt.Errorf("%w: Accelerator requires 127.0.0.1", ErrInvalidListenAddress)
		}
		return nil
	}
	if host != "" {
		return fmt.Errorf("%w: compatibility mode requires wildcard host", ErrInvalidListenAddress)
	}
	return nil
}
