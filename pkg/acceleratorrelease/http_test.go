package acceleratorrelease

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientBoundsAndHeaders(t *testing.T) {
	client := newHTTPClient()
	if client.Timeout != 10*time.Second || client.Jar != nil {
		t.Fatalf("client bounds/jar = %v, %#v", client.Timeout, client.Jar)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil || transport.TLSHandshakeTimeout != 5*time.Second || transport.ResponseHeaderTimeout != 5*time.Second {
		t.Fatalf("transport bounds = %#v", transport)
	}
	if httpDialTimeout != 3*time.Second || maxRedirects != 3 {
		t.Fatal("dial/redirect constants drifted")
	}
	var captured *http.Request
	doer := doerFunc(func(request *http.Request) (*http.Response, error) {
		captured = request
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	if body, class := fetchAsset(context.Background(), doer, "https://github.com/exact", 2); class != fetchOK || string(body) != "ok" {
		t.Fatalf("fetch = %q, %v", body, class)
	}
	if captured.Method != http.MethodGet || captured.Header.Get("Accept") != "application/octet-stream" || captured.Header.Get("User-Agent") != "kubikles-accelerator-release" {
		t.Fatalf("request contract = %#v", captured)
	}
	for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie"} {
		if captured.Header.Get(header) != "" {
			t.Fatalf("request carried %s", header)
		}
	}
}

func TestRedirectPolicy(t *testing.T) {
	client := newHTTPClient()
	request := func(rawURL string) *http.Request {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Request{URL: u, Header: make(http.Header)}
	}
	for hops := 0; hops <= 3; hops++ {
		via := make([]*http.Request, hops)
		for i := range via {
			via[i] = request("https://github.com/previous")
		}
		redirect := request("https://release-assets.githubusercontent.com:443/asset?signature=secret")
		for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Referer"} {
			redirect.Header.Set(header, "must-not-survive")
		}
		if err := client.CheckRedirect(redirect, via); err != nil {
			t.Fatalf("%d allowed hops rejected: %v", hops, err)
		}
		for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Referer"} {
			if got := redirect.Header.Get(header); got != "" {
				t.Fatalf("%s survived redirect: %q", header, got)
			}
		}
	}
	four := []*http.Request{request("https://github.com/1"), request("https://github.com/2"), request("https://github.com/3"), request("https://github.com/4")}
	if err := client.CheckRedirect(request("https://github.com/fifth"), four); !errors.Is(err, errRedirectPolicy) {
		t.Fatalf("fourth redirect accepted: %v", err)
	}
	for _, rawURL := range []string{
		"http://github.com/asset", "https://example.com/asset", "https://sub.github.com/asset",
		"https://user@github.com/asset", "https://github.com:444/asset",
	} {
		if err := client.CheckRedirect(request(rawURL), nil); !errors.Is(err, errRedirectPolicy) {
			t.Fatalf("unsafe redirect accepted: %s (%v)", rawURL, err)
		}
	}
}

type closeTrackingBody struct {
	reader   io.Reader
	readErr  error
	closeErr error
	closed   bool
}

func (b *closeTrackingBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}
func (b *closeTrackingBody) Close() error { b.closed = true; return b.closeErr }

func TestFetchAssetBoundsAndCloses(t *testing.T) {
	for _, size := range []int{maxChecksumBytes, maxDescriptorBytes} {
		body := &closeTrackingBody{reader: bytes.NewReader(bytes.Repeat([]byte("a"), size))}
		doer := doerFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil })
		got, class := fetchAsset(context.Background(), doer, "https://github.com/asset", size)
		if class != fetchOK || len(got) != size || !body.closed {
			t.Fatalf("exact limit %d = %d, %v, closed=%v", size, len(got), class, body.closed)
		}
		body = &closeTrackingBody{reader: bytes.NewReader(bytes.Repeat([]byte("a"), size+1))}
		_, class = fetchAsset(context.Background(), doer, "https://github.com/asset", size)
		if class != fetchIntegrity || !body.closed {
			t.Fatalf("limit+1 %d = %v, closed=%v", size, class, body.closed)
		}
	}
	for _, status := range []int{404, 410} {
		body := &closeTrackingBody{reader: strings.NewReader("secret body")}
		_, class := fetchAsset(context.Background(), doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: body}, nil
		}), "https://github.com/asset", 10)
		if class != fetchMissing || !body.closed {
			t.Fatalf("status %d = %v closed=%v", status, class, body.closed)
		}
	}
	for _, status := range []int{408, 425, 429, 403, 500, 503, 302} {
		body := &closeTrackingBody{reader: strings.NewReader("secret body")}
		_, class := fetchAsset(context.Background(), doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: body}, nil
		}), "https://github.com/asset", 10)
		if class != fetchTransient || !body.closed {
			t.Fatalf("status %d = %v closed=%v", status, class, body.closed)
		}
	}
	for name, err := range map[string]error{"cancel": context.Canceled, "timeout": context.DeadlineExceeded, "dns": &net.DNSError{Err: "secret", Name: "secret.example"}} {
		t.Run(name, func(t *testing.T) {
			_, class := fetchAsset(context.Background(), doerFunc(func(*http.Request) (*http.Response, error) { return nil, err }), "https://github.com/asset", 10)
			if class != fetchTransient {
				t.Fatalf("class = %v", class)
			}
		})
	}
	_, class := fetchAsset(context.Background(), doerFunc(func(*http.Request) (*http.Response, error) { return nil, &url.Error{Err: errRedirectPolicy} }), "https://github.com/asset", 10)
	if class != fetchIntegrity {
		t.Fatalf("redirect error = %v", class)
	}
	for _, body := range []*closeTrackingBody{{reader: strings.NewReader("x"), readErr: errors.New("read secret")}, {reader: strings.NewReader("x"), closeErr: errors.New("close secret")}} {
		_, class := fetchAsset(context.Background(), doerFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil }), "https://github.com/asset", 10)
		if class != fetchTransient || !body.closed {
			t.Fatalf("body failure = %v closed=%v", class, body.closed)
		}
	}
}
