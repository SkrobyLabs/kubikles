package main

import (
	"context"
	"fmt"
	"log"
	"time"

	v1 "k8s.io/api/core/v1"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

func (a *App) listHelmReleaseMetadata(requestID, namespace string) ([]acceleratorsecret.HelmReleaseMetadata, error) {
	if a == nil || a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if requestID != "" {
		var sequence int64
		ctx, sequence = a.listRequestManager.StartRequest(requestID)
		defer a.listRequestManager.CompleteRequest(requestID, sequence)
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
	}
	secrets, err := a.k8sClient.ListHelmReleaseSecretsWithContext(ctx, namespace)
	if err == k8s.ErrRequestCancelled {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer clearHelmReleaseSecretPayloads(secrets)
	releases, failures := acceleratorsecret.ProjectLatestHelmReleaseSecrets(secrets)
	fields := map[string]interface{}{"namespace": namespace, "storageSecrets": len(secrets), "releases": len(releases), "invalidLatestSecrets": failures}
	if a.runtimeMode == RuntimeModeAccelerator {
		log.Printf("Accelerator Helm release metadata projected namespace=%q storageSecrets=%d releases=%d invalidLatestSecrets=%d", namespace, len(secrets), len(releases), failures)
	} else {
		debug.LogHelm("ListHelmReleaseMetadata", fields)
	}
	return releases, nil
}

func clearHelmReleaseSecretPayloads(secrets []v1.Secret) {
	for index := range secrets {
		for key, value := range secrets[index].Data {
			clear(value)
			delete(secrets[index].Data, key)
		}
	}
	clear(secrets)
}
