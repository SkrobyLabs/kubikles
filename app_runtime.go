package main

import (
	"context"
	"errors"
	"fmt"

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

// AgentRouter reserves capability-based routing for a future desktop integration.
type AgentRouter interface {
	Supports(agent.Capability) bool
}

// NoopAgentRouter supports no capabilities.
type NoopAgentRouter struct{}

func (NoopAgentRouter) Supports(agent.Capability) bool { return false }

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
