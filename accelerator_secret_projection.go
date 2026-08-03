package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

// acceleratorSecretCaller restricts only the three Secret reads whose response
// shape differs at the Accelerator boundary. All other calls are delegated byte
// for byte to the generated caller.
type acceleratorSecretCaller struct {
	delegate server.MethodCaller
	app      *App
}

func newAcceleratorSecretCaller(delegate server.MethodCaller, app *App) server.MethodCaller {
	return &acceleratorSecretCaller{delegate: delegate, app: app}
}

func (c *acceleratorSecretCaller) CallMethod(call agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	switch method {
	case "ListSecretsMetadata":
		requestID, namespace, err := acceleratorSecretListArguments(args)
		if err != nil {
			return nil, err
		}
		hideHelm, err := acceleratorHideHelmOption(args)
		if err != nil {
			return nil, err
		}
		return c.app.acceleratorSecretsMetadata(requestID, namespace, k8s.SecretListOptions{ExcludeHelmReleases: hideHelm})
	case "GetSecretData":
		namespace, name, err := acceleratorSecretDetailArguments(args)
		if err != nil {
			return nil, err
		}
		return c.app.acceleratorSecretData(namespace, name)
	case "GetSecretYaml":
		namespace, name, err := acceleratorSecretDetailArguments(args)
		if err != nil {
			return nil, err
		}
		return c.app.acceleratorSecretYAML(namespace, name)
	default:
		return c.delegate.CallMethod(call, method, args)
	}
}

func acceleratorSecretListArguments(args []json.RawMessage) (string, string, error) {
	requestID, err := acceleratorStringArgument(args, 0)
	if err != nil {
		return "", "", err
	}
	namespace, err := acceleratorStringArgument(args, 1)
	if err != nil {
		return "", "", err
	}
	return requestID, namespace, nil
}

func acceleratorSecretDetailArguments(args []json.RawMessage) (string, string, error) {
	namespace, err := acceleratorStringArgument(args, 0)
	if err != nil {
		return "", "", err
	}
	name, err := acceleratorStringArgument(args, 1)
	if err != nil {
		return "", "", err
	}
	return namespace, name, nil
}

func acceleratorStringArgument(args []json.RawMessage, index int) (string, error) {
	if index >= len(args) || args[index] == nil {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(args[index], &value); err != nil {
		return "", fmt.Errorf("argument %d: %w", index, err)
	}
	return value, nil
}

// acceleratorHideHelmOption intentionally distinguishes an absent option from
// JSON null: absent keeps the secure default, while null explicitly disables it.
func acceleratorHideHelmOption(args []json.RawMessage) (bool, error) {
	if len(args) <= 2 || args[2] == nil {
		return true, nil
	}
	if bytes.Equal(bytes.TrimSpace(args[2]), []byte("null")) {
		return false, nil
	}
	var hide bool
	if err := json.Unmarshal(args[2], &hide); err != nil {
		return false, fmt.Errorf("argument 2: %w", err)
	}
	return hide, nil
}

func (a *App) acceleratorSecretData(namespace, name string) ([]k8s.DataEntry, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	entries, err := a.k8sClient.GetSecretData(namespace, name)
	if err != nil {
		return nil, err
	}
	projected := make([]k8s.DataEntry, len(entries))
	copy(projected, entries)
	sort.Slice(projected, func(i, j int) bool { return projected[i].Key < projected[j].Key })
	return projected, nil
}

type acceleratorSecretListItem = acceleratorsecret.SecretListItem

// projectAcceleratorSecretListItem is the single closed 20A projection used by
// both list responses and 20B watch events. The ordinary desktop DTO remains
// untouched and may retain its richer metadata fields.
func projectAcceleratorSecretListItem(item k8s.SecretListItem) acceleratorSecretListItem {
	return acceleratorsecret.ProjectSecretListItem(item)
}

func (a *App) acceleratorSecretYAML(namespace, name string) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretYaml(namespace, name)
}

func (a *App) acceleratorSecretsMetadata(requestID, namespace string, options k8s.SecretListOptions) ([]acceleratorSecretListItem, error) {
	if a.k8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}
	var items []k8s.SecretListItem
	var err error
	if requestID != "" {
		ctx, sequence := a.listRequestManager.StartRequest(requestID)
		defer a.listRequestManager.CompleteRequest(requestID, sequence)
		items, err = a.k8sClient.ListSecretsMetadataWithOptions(ctx, namespace, options, nil)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		items, err = a.k8sClient.ListSecretsMetadataWithOptions(ctx, namespace, options)
	}
	if err == k8s.ErrRequestCancelled {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	projected := make([]acceleratorSecretListItem, len(items))
	for i, item := range items {
		projected[i] = projectAcceleratorSecretListItem(item)
	}
	return projected, nil
}
