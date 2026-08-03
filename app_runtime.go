package main

import (
	"context"
	"errors"
	"fmt"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
)

// RuntimeMode identifies the process composition used to construct an App.
type RuntimeMode string

const (
	RuntimeModeDesktop     RuntimeMode = "desktop"
	RuntimeModeServer      RuntimeMode = "server"
	RuntimeModeAccelerator RuntimeMode = "accelerator"
)

var (
	ErrInvalidRuntimeMode               = errors.New("invalid runtime mode")
	ErrAcceleratorClientFactoryRequired = errors.New("Accelerator Kubernetes client factory required")
	ErrAcceleratorClientInitialization  = errors.New("Accelerator Kubernetes client initialization failed")
)

// KubernetesClientFactory constructs the Kubernetes client for an App.
type KubernetesClientFactory func() (*k8s.Client, error)

// SecretReadSourceToken is opaque process-local correlation for one published
// Secret source. It is not authority and contains no Accelerator identity.
type SecretReadSourceToken string

var ErrIntegratedSecretReadsUnavailable = errors.New("Secret reads unavailable")

// SecretReadRouter is the closed desktop integration surface. It deliberately
// exposes only the six Secret read/lifecycle operations authorized by policy.
type SecretReadRouter interface {
	Retain(context.Context, string)
	Release()
	FenceContextSwitch(string)
	ContextSwitched(string, bool)
	ListSecretsMetadata(context.Context, SecretReadSourceToken, string, string, bool) ([]k8s.SecretListItem, error)
	GetSecretData(context.Context, SecretReadSourceToken, string, string) ([]k8s.DataEntry, error)
	GetSecretYaml(context.Context, SecretReadSourceToken, string, string) (string, error)
	CancelListRequest(context.Context, SecretReadSourceToken, string) (bool, error)
	SubscribeSecretWatcher(context.Context, SecretReadSourceToken, string, bool) (acceleratorsecret.SecretWatchSubscription, error)
	UnsubscribeSecretWatcher(context.Context, SecretReadSourceToken, acceleratorsecret.SecretWatchSpecID) error
	Quiesce(context.Context)
	StopProducers(context.Context)
	Close(context.Context)
}

// AgentRouter keeps capability discovery separate from the exact Secret seam.
type AgentRouter interface {
	Supports(agent.Capability) bool
	SecretReads() SecretReadRouter
}

// NoopAgentRouter supports no capabilities.
type NoopAgentRouter struct{}

func (NoopAgentRouter) Supports(agent.Capability) bool { return false }
func (NoopAgentRouter) SecretReads() SecretReadRouter  { return unavailableSecretReadRouter{} }

type secretReadAgentRouter struct {
	base    AgentRouter
	secrets SecretReadRouter
}

func (r secretReadAgentRouter) Supports(capability agent.Capability) bool {
	return r.base != nil && r.base.Supports(capability)
}
func (r secretReadAgentRouter) SecretReads() SecretReadRouter {
	if r.secrets == nil {
		return unavailableSecretReadRouter{}
	}
	return r.secrets
}

type unavailableSecretReadRouter struct{}

func (unavailableSecretReadRouter) Retain(context.Context, string) {}
func (unavailableSecretReadRouter) Release()                       {}
func (unavailableSecretReadRouter) FenceContextSwitch(string)      {}
func (unavailableSecretReadRouter) ContextSwitched(string, bool)   {}
func (unavailableSecretReadRouter) Quiesce(context.Context)        {}
func (unavailableSecretReadRouter) StopProducers(context.Context)  {}
func (unavailableSecretReadRouter) Close(context.Context)          {}
func (unavailableSecretReadRouter) ListSecretsMetadata(context.Context, SecretReadSourceToken, string, string, bool) ([]k8s.SecretListItem, error) {
	return nil, ErrIntegratedSecretReadsUnavailable
}
func (unavailableSecretReadRouter) GetSecretData(context.Context, SecretReadSourceToken, string, string) ([]k8s.DataEntry, error) {
	return nil, ErrIntegratedSecretReadsUnavailable
}
func (unavailableSecretReadRouter) GetSecretYaml(context.Context, SecretReadSourceToken, string, string) (string, error) {
	return "", ErrIntegratedSecretReadsUnavailable
}
func (unavailableSecretReadRouter) CancelListRequest(context.Context, SecretReadSourceToken, string) (bool, error) {
	return false, ErrIntegratedSecretReadsUnavailable
}
func (unavailableSecretReadRouter) SubscribeSecretWatcher(context.Context, SecretReadSourceToken, string, bool) (acceleratorsecret.SecretWatchSubscription, error) {
	return acceleratorsecret.SecretWatchSubscription{}, ErrIntegratedSecretReadsUnavailable
}
func (unavailableSecretReadRouter) UnsubscribeSecretWatcher(context.Context, SecretReadSourceToken, acceleratorsecret.SecretWatchSpecID) error {
	return ErrIntegratedSecretReadsUnavailable
}

// RuntimeLifecycle participates in the ordered App shutdown phases.
type RuntimeLifecycle interface {
	Quiesce(context.Context)
	StopProducers(context.Context)
	Close(context.Context)
}

// NoopRuntimeLifecycle has no external runtime resources.
type NoopRuntimeLifecycle struct{}

func (NoopRuntimeLifecycle) Quiesce(context.Context)       {}
func (NoopRuntimeLifecycle) StopProducers(context.Context) {}
func (NoopRuntimeLifecycle) Close(context.Context)         {}

// AppOptions contains the explicit App composition seams.
type AppOptions struct {
	Mode                    RuntimeMode
	KubernetesClientFactory KubernetesClientFactory
	AgentRouter             AgentRouter
	Lifecycle               RuntimeLifecycle
}

func normalizeAppOptions(options AppOptions) (AppOptions, error) {
	switch options.Mode {
	case RuntimeModeDesktop, RuntimeModeServer, RuntimeModeAccelerator:
	default:
		return AppOptions{}, fmt.Errorf("%w: %q", ErrInvalidRuntimeMode, options.Mode)
	}
	if options.KubernetesClientFactory == nil {
		if options.Mode == RuntimeModeAccelerator {
			return AppOptions{}, ErrAcceleratorClientFactoryRequired
		}
		options.KubernetesClientFactory = k8s.NewClient
	}
	if options.AgentRouter == nil {
		options.AgentRouter = NoopAgentRouter{}
	}
	if options.Lifecycle == nil {
		options.Lifecycle = NoopRuntimeLifecycle{}
	}
	return options, nil
}
