package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kubikles/pkg/compressedassets"
)

func (s *Server) buildHandler() (http.Handler, error) {
	static, err := s.staticHandler()
	if err != nil {
		return nil, err
	}
	if s.options.BoundaryMode == BoundaryModeAccelerator {
		return s.acceleratorHandler(static), nil
	}
	return s.compatibilityHandler(static), nil
}

func (s *Server) staticHandler() (http.Handler, error) {
	subFS, err := fs.Sub(s.assets, "frontend/dist")
	if err != nil {
		return nil, fmt.Errorf("failed to create sub filesystem: %w", err)
	}
	return compressedassets.GzipAwareFileServer(subFS), nil
}

func (s *Server) compatibilityHandler(static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", s.handleLive)
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/api/", s.handleAPI)
	mux.Handle("/", static)
	return mux
}

func (s *Server) acceleratorHandler(static http.Handler) http.Handler {
	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/call", s.handleCanonicalAPI)
	if hasAcceleratorInfoProvider(s.options.AcceleratorInfoProvider) {
		protected.HandleFunc("GET /api/accelerator-info", s.handleAcceleratorInfo)
	}
	if s.options.BrowserSessions != nil {
		protected.Handle("POST /api/accelerator-browser-ticket", requireCreator(http.HandlerFunc(s.handleMintBrowserTicket)))
		protected.Handle("POST /api/accelerator-browser-session/revoke", requireCreator(http.HandlerFunc(s.handleRevokeBrowserSession)))
	}
	canonical := s.options.ProtectedRouteGuard(protected)
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !exactRequestPath(r, r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/accelerator-info" && !hasAcceleratorInfoProvider(s.options.AcceleratorInfoProvider) {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/livez":
			s.handleLive(w, r)
		case "/readyz":
			s.handleReady(w, r)
		case "/api/call", "/api/accelerator-info":
			wantMethod := http.MethodPost
			if r.URL.Path == "/api/accelerator-info" {
				wantMethod = http.MethodGet
			}
			if r.Method != wantMethod {
				w.Header().Set("Allow", wantMethod)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if s.quiescing.Load() {
				s.writeError(w, http.StatusServiceUnavailable, "unavailable")
				return
			}
			canonical.ServeHTTP(w, r)
		case "/api/accelerator-browser-session":
			if s.options.BrowserSessions == nil {
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if s.quiescing.Load() {
				s.writeError(w, http.StatusServiceUnavailable, "unavailable")
				return
			}
			s.handleExchangeBrowserSession(w, r)
		case "/api/accelerator-browser-ticket", "/api/accelerator-browser-session/revoke":
			if s.options.BrowserSessions == nil {
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if s.quiescing.Load() {
				s.writeError(w, http.StatusServiceUnavailable, "unavailable")
				return
			}
			canonical.ServeHTTP(w, r)
		case "/ws":
			if s.options.AcceleratorWebSocketAuthenticator == nil || r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			if s.quiescing.Load() {
				s.writeError(w, http.StatusServiceUnavailable, "unavailable")
				return
			}
			release, admitted := s.options.AcceleratorSessions.beginUpgrade()
			if !admitted {
				s.writeError(w, http.StatusServiceUnavailable, "unavailable")
				return
			}
			if s.quiescing.Load() {
				release()
				s.writeError(w, http.StatusServiceUnavailable, "unavailable")
				return
			}
			s.options.AcceleratorWebSocketAuthenticator.handleAdmitted(w, r, release)
		default:
			if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/ws") {
				http.NotFound(w, r)
				return
			}
			static.ServeHTTP(w, r)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status := validateAcceleratorRequest(r); status != 0 {
			http.Error(w, http.StatusText(status), status)
			return
		}
		router.ServeHTTP(w, r)
	})
}

func exactRequestPath(r *http.Request, path string) bool {
	return r.URL.RawPath == "" || r.URL.RawPath == path
}

type normalizedAuthority struct {
	host    string
	port    string
	hasPort bool
}

func parseLoopbackAuthority(authority string) (normalizedAuthority, error) {
	if authority == "" || strings.ContainsAny(authority, " ,/@\\?#") {
		return normalizedAuthority{}, errors.New("invalid authority")
	}
	host := authority
	port := ""
	hasPort := false
	if strings.HasPrefix(authority, "[") {
		closing := strings.IndexByte(authority, ']')
		if closing < 0 {
			return normalizedAuthority{}, errors.New("invalid bracketed host")
		}
		host = authority[1:closing]
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() != nil {
			return normalizedAuthority{}, errors.New("bracketed host is not IPv6")
		}
		remainder := authority[closing+1:]
		if remainder != "" {
			if !strings.HasPrefix(remainder, ":") || len(remainder) == 1 {
				return normalizedAuthority{}, errors.New("invalid port")
			}
			port, hasPort = remainder[1:], true
		}
	} else if strings.Count(authority, ":") == 1 {
		var err error
		host, port, err = net.SplitHostPort(authority)
		if err != nil {
			return normalizedAuthority{}, err
		}
		hasPort = true
	} else if strings.Contains(authority, ":") {
		return normalizedAuthority{}, errors.New("unbracketed IPv6 host")
	}
	if hasPort {
		value, err := strconv.Atoi(port)
		if err != nil || port == "" || strings.IndexFunc(port, func(r rune) bool { return r < '0' || r > '9' }) >= 0 || value < 0 || value > 65535 {
			return normalizedAuthority{}, errors.New("invalid port")
		}
	}
	if strings.EqualFold(host, "localhost") {
		return normalizedAuthority{host: "localhost", port: port, hasPort: hasPort}, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return normalizedAuthority{}, errors.New("host is not loopback")
	}
	return normalizedAuthority{host: ip.String(), port: port, hasPort: hasPort}, nil
}

func validateAcceleratorRequest(r *http.Request) int {
	host, err := parseLoopbackAuthority(r.Host)
	if err != nil {
		return http.StatusBadRequest
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return 0
	}
	if len(origins) != 1 || origins[0] == "" || strings.ContainsAny(origins[0], ",#") {
		return http.StatusBadRequest
	}
	origin, err := url.Parse(origins[0])
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.User != nil || origin.Opaque != "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery {
		return http.StatusBadRequest
	}
	if origin.Scheme != "http" {
		return http.StatusForbidden
	}
	originHost, err := parseLoopbackAuthority(origin.Host)
	if err != nil {
		return http.StatusForbidden
	}
	if host != originHost {
		return http.StatusForbidden
	}
	return 0
}

func (s *Server) handleCanonicalAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	callContext, authenticated := authenticatedCreatorContext(r.Context())
	if !authenticated {
		writeAcceleratorError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, AcceleratorMaxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	var request APIRequest
	if err := decoder.Decode(&request); err != nil {
		s.writeCanonicalDecodeError(w, err)
		return
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			s.writeError(w, http.StatusBadRequest, "invalid request")
		} else {
			s.writeCanonicalDecodeError(w, err)
		}
		return
	}
	if request.Method == "" {
		s.writeError(w, http.StatusBadRequest, "method name required")
		return
	}
	if s.options.MethodAuthorizer == nil || !s.options.MethodAuthorizer.Authorize(callContext, request.Method) {
		writeAcceleratorError(w, http.StatusForbidden, "forbidden")
		return
	}
	result, err := s.caller.CallMethod(callContext, request.Method, request.Args)
	if err != nil {
		writeAcceleratorError(w, http.StatusInternalServerError, "internal error")
		return
	}
	_ = writeJSON(w, map[string]interface{}{"data": result})
}

func writeAcceleratorError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = writeJSON(w, map[string]string{"error": message})
}

func (s *Server) writeCanonicalDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		s.writeError(w, http.StatusRequestEntityTooLarge, "request too large")
		return
	}
	s.writeError(w, http.StatusBadRequest, "invalid request")
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeHealth(w, http.StatusOK, "live\n")
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.quiescing.Load() || s.options.ReadinessProvider == nil {
		writeHealth(w, http.StatusServiceUnavailable, "not ready\n")
		return
	}
	ctx, cancel := contextWithReadinessTimeout(r)
	defer cancel()
	if err := s.options.ReadinessProvider.Ready(ctx); err != nil {
		writeHealth(w, http.StatusServiceUnavailable, "not ready\n")
		return
	}
	if s.quiescing.Load() {
		writeHealth(w, http.StatusServiceUnavailable, "not ready\n")
		return
	}
	writeHealth(w, http.StatusOK, "ready\n")
}

func contextWithReadinessTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 2*time.Second)
}

func writeHealth(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func writeJSON(w http.ResponseWriter, value interface{}) error {
	return json.NewEncoder(w).Encode(value)
}
