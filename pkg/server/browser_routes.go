package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const BrowserExchangeMaxBodyBytes int64 = 4096

type browserTicketResponse struct {
	Ticket        string `json:"ticket"`
	LaunchReceipt string `json:"launchReceipt,omitempty"`
	ExpiresAt     string `json:"expiresAt"`
}
type browserExchangeRequest struct {
	Ticket string `json:"ticket"`
}
type browserBearerResponse struct {
	Bearer    string `json:"bearer"`
	ExpiresAt string `json:"expiresAt"`
}

func (v browserTicketResponse) String() string              { return "<redacted>" }
func (v browserExchangeRequest) String() string             { return "<redacted>" }
func (v browserBearerResponse) String() string              { return "<redacted>" }
func (v browserTicketResponse) Format(s fmt.State, _ rune)  { _, _ = io.WriteString(s, "<redacted>") }
func (v browserExchangeRequest) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "<redacted>") }
func (v browserBearerResponse) Format(s fmt.State, _ rune)  { _, _ = io.WriteString(s, "<redacted>") }

func requireCreator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if authenticatedCredentialKind(r.Context()) != credentialKindCreator {
			writeAcceleratorError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	}
}
func (s *Server) handleMintBrowserTicket(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		s.writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !readEmptyBrowserRequestBody(w, r) {
		s.writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if s.options.BrowserEntryAvailability == nil || !s.options.BrowserEntryAvailability.BrowserEntryEnabled() {
		writeAcceleratorError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	call, ok := authenticatedCreatorContext(r.Context())
	if !ok {
		writeAcceleratorError(w, http.StatusForbidden, "forbidden")
		return
	}
	ticket, receipt, expiry, err := s.options.BrowserSessions.mintForCreator(call)
	if err != nil {
		writeAcceleratorError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = writeJSON(w, browserTicketResponse{Ticket: ticket.encoded(), LaunchReceipt: receipt.encoded(), ExpiresAt: expiry.Format(time.RFC3339Nano)})
}
func (s *Server) handleExchangeBrowserSession(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Header.Values("Authorization")) != 0 || len(r.Header.Values("Cookie")) != 0 {
		s.writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, BrowserExchangeMaxBodyBytes)
	var req browserExchangeRequest
	d := json.NewDecoder(r.Body)
	if err := decodeExactBrowserExchangeRequest(d, &req); err != nil {
		s.writeBrowserExchangeDecodeError(w, err)
		return
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		if err == nil {
			s.writeError(w, http.StatusBadRequest, "invalid request")
		} else {
			s.writeBrowserExchangeDecodeError(w, err)
		}
		return
	}
	ticket, err := ParseBrowserTicket(req.Ticket)
	if err != nil {
		writeAcceleratorError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	bearer, expiry, err := s.options.BrowserSessions.Exchange(ticket)
	if err != nil {
		if errors.Is(err, ErrInvalidBrowserTicket) {
			writeAcceleratorError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeAcceleratorError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, browserBearerResponse{Bearer: bearer.encoded(), ExpiresAt: expiry.Format(time.RFC3339Nano)})
}

func decodeExactBrowserExchangeRequest(d *json.Decoder, req *browserExchangeRequest) error {
	first, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := first.(json.Delim)
	if !ok || delim != '{' {
		return errors.New("invalid request")
	}
	if !d.More() {
		return errors.New("invalid request")
	}
	name, err := d.Token()
	if err != nil || name != "ticket" {
		return errors.New("invalid request")
	}
	var raw json.RawMessage
	if err := d.Decode(&raw); err != nil {
		return err
	}
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &req.Ticket) != nil {
		return errors.New("invalid request")
	}
	if d.More() {
		return errors.New("invalid request")
	}
	last, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok = last.(json.Delim)
	if !ok || delim != '}' {
		return errors.New("invalid request")
	}
	return nil
}
func (s *Server) writeBrowserExchangeDecodeError(w http.ResponseWriter, err error) {
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		s.writeError(w, http.StatusRequestEntityTooLarge, "request too large")
		return
	}
	s.writeError(w, http.StatusBadRequest, "invalid request")
}
func (s *Server) handleRevokeBrowserSession(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		s.writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !readEmptyBrowserRequestBody(w, r) {
		s.writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	s.options.BrowserSessions.Revoke(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func readEmptyBrowserRequestBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1)
	body, err := io.ReadAll(r.Body)
	return err == nil && len(body) == 0
}
