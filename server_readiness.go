package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"

	"k8s.io/client-go/rest"
	"kubikles/pkg/k8s"
)

var (
	errAppNotInitialized     = errors.New("App is not initialized")
	errKubernetesUnavailable = errors.New("Kubernetes API is unavailable")
)

type kubernetesReachabilityProbe func(context.Context, *k8s.Client) error

type appReadinessProvider struct {
	app         *App
	initialized atomic.Bool
	probe       kubernetesReachabilityProbe
}

func newAppReadinessProvider(app *App) *appReadinessProvider {
	return &appReadinessProvider{app: app, probe: newKubernetesReachabilityProbe()}
}

func (p *appReadinessProvider) markInitialized() {
	p.initialized.Store(true)
}

func (p *appReadinessProvider) Ready(ctx context.Context) error {
	if !p.initialized.Load() {
		return errAppNotInitialized
	}
	if p.app == nil || p.app.k8sInitError != nil || p.app.k8sClient == nil || p.probe == nil {
		return errKubernetesUnavailable
	}
	if err := p.probe(ctx, p.app.k8sClient); err != nil {
		return fmt.Errorf("%w: %w", errKubernetesUnavailable, err)
	}
	return nil
}

func newKubernetesReachabilityProbe() kubernetesReachabilityProbe {
	return probeKubernetesReachability
}

func probeKubernetesReachability(ctx context.Context, client *k8s.Client) error {
	config, err := client.GetRestConfigForContext("")
	if err != nil {
		return err
	}
	return probeRESTConfigReachability(ctx, config)
}

func probeRESTConfigReachability(ctx context.Context, config *rest.Config) error {
	if config == nil {
		return errors.New("nil REST config")
	}
	sharedHTTPClient, err := rest.HTTPClientFor(rest.CopyConfig(config))
	if err != nil {
		return fmt.Errorf("create Kubernetes HTTP client: %w", err)
	}
	httpClient := *sharedHTTPClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	endpoint, err := url.JoinPath(config.Host, "version")
	if err != nil {
		return fmt.Errorf("join Kubernetes version URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Kubernetes version request: %w", err)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request Kubernetes version: %w", err)
	}
	defer response.Body.Close()
	_, discardErr := io.Copy(io.Discard, response.Body)
	if discardErr != nil {
		return fmt.Errorf("discard Kubernetes version response: %w", discardErr)
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("Kubernetes version status %d", response.StatusCode)
	}
	return nil
}
