package main

import "testing"

func TestAcceleratorKubernetesClientFactorySignature(t *testing.T) {
	var factory KubernetesClientFactory = acceleratorKubernetesClientFactory
	if factory == nil {
		t.Fatal("accelerator Kubernetes client factory is nil")
	}
}
