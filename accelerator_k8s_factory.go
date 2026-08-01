package main

import "kubikles/pkg/k8s"

var _ KubernetesClientFactory = acceleratorKubernetesClientFactory

func acceleratorKubernetesClientFactory() (*k8s.Client, error) {
	return k8s.NewInClusterClient()
}
