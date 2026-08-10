//go:build !accelerator

package main

import (
	"context"

	"kubikles/pkg/helm"
)

// HelmReleaseReadRouter extends the Secret read route with one list-only
// projection over Helm's Secret storage. It exposes no Helm mutations or
// release-detail operations.
type HelmReleaseReadRouter interface {
	ListHelmReleaseMetadata(context.Context, SecretReadSourceToken, string, string) ([]helm.Release, error)
}
