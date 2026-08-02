package acceleratorrelease

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	httpTotalTimeout          = 10 * time.Second
	httpDialTimeout           = 3 * time.Second
	httpTLSHandshakeTimeout   = 5 * time.Second
	httpResponseHeaderTimeout = 5 * time.Second
	maxRedirects              = 3
	maxChecksumBytes          = 256
	maxDescriptorBytes        = 64 << 10
)

var errRedirectPolicy = errors.New("accelerator release redirect policy")

type fetchClass int

const (
	fetchOK fetchClass = iota
	fetchMissing
	fetchTransient
	fetchIntegrity
)

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: httpDialTimeout}).DialContext
	transport.TLSHandshakeTimeout = httpTLSHandshakeTimeout
	transport.ResponseHeaderTimeout = httpResponseHeaderTimeout
	return &http.Client{
		Transport: transport,
		Timeout:   httpTotalTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			stripSensitiveHeaders(request.Header)
			if len(via) > maxRedirects || !allowedRedirectURL(request.URL) {
				return errRedirectPolicy
			}
			return nil
		},
	}
}

func stripSensitiveHeaders(header http.Header) {
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Referer"} {
		header.Del(name)
	}
}

func allowedRedirectURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	if port := u.Port(); port != "" && port != "443" {
		return false
	}
	host := u.Hostname()
	return host == "github.com" || host == "release-assets.githubusercontent.com"
}

func fetchAsset(ctx context.Context, client httpDoer, rawURL string, limit int) ([]byte, fetchClass) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fetchIntegrity
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "kubikles-accelerator-release")
	response, err := client.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if errors.Is(err, errRedirectPolicy) {
			return nil, fetchIntegrity
		}
		return nil, fetchTransient
	}
	if response == nil || response.Body == nil {
		return nil, fetchTransient
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		_ = response.Body.Close()
		return nil, fetchMissing
	}
	if response.StatusCode < http.StatusOK || response.StatusCode > 299 {
		_ = response.Body.Close()
		return nil, fetchTransient
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	closeErr := response.Body.Close()
	// A limit breach is contradictory successful evidence. A failed body, on the
	// other hand, never gave us complete bytes to trust, so it remains eligible
	// for the exact cached pair.
	if len(body) > limit {
		return nil, fetchIntegrity
	}
	if readErr != nil || closeErr != nil {
		return nil, fetchTransient
	}
	return body, fetchOK
}
