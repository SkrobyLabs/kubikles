//go:build helm && accelerator_provision_kind

package helm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

var acceleratorRegistrySeamMu sync.Mutex

// SetAcceleratorRegistryHTTPClientForTest is compiled only into the disposable
// Kind smoke. It changes transport delivery only: PullAcceleratorChart still
// receives and validates the fixed production GHCR digest reference.
func SetAcceleratorRegistryTransportForTest(factory func(http.RoundTripper) http.RoundTripper) func() {
	acceleratorRegistrySeamMu.Lock()
	previous := acceleratorRegistryTransportForTest
	acceleratorRegistryTransportForTest = factory
	return func() {
		acceleratorRegistryTransportForTest = previous
		acceleratorRegistrySeamMu.Unlock()
	}
}

// AcceleratorSweepProofStageForTest returns only a fixed stage name and is
// compiled solely into the disposable Kind harness.
func (c *Client) AcceleratorSweepProofStageForTest(ctx context.Context, config *rest.Config, namespace, name string) string {
	actionConfig, err := acceleratorActionConfig(ctx, config, namespace)
	if err != nil {
		return "action_config"
	}
	client, err := acceleratorClientset(ctx, config)
	if err != nil {
		return "client"
	}
	history, err := actionConfig.Releases.History(name)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return "release_absent"
	}
	if err != nil {
		return "history_error"
	}
	if len(history) != 1 {
		return "history_count"
	}
	if _, _, _, _, stage := proveStoredSweepReleaseStage(history[0], namespace, name); stage != "" {
		return "stored_" + stage
	}
	storage, err := client.CoreV1().Secrets(namespace).Get(ctx, acceleratorStorageName(name), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "storage_absent"
	}
	if err != nil || storage.UID == "" {
		return "storage_error"
	}
	return "stored_ok"
}

// ListReleasesForAcceleratorProvisionKind is a context-bound final assertion
// seam compiled only into the disposable Kind harness. The production List
// API has no caller context, while this path makes every Helm storage request
// inherit the harness phase deadline.
func (c *Client) ListReleasesForAcceleratorProvisionKind(ctx context.Context, config *rest.Config, namespace string) ([]Release, error) {
	actionConfig, err := acceleratorActionConfig(ctx, config, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize accelerator Kind Helm assertion: %w", err)
	}
	listAction := action.NewList(actionConfig)
	listAction.All = true
	listAction.SetStateMask()
	results, err := listAction.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list accelerator Kind Helm assertions: %w", err)
	}
	releases := make([]Release, 0, len(results))
	for _, result := range results {
		releases = append(releases, releaseToModel(result))
	}
	return releases, nil
}
