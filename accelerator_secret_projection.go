package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	entries, err := a.GetSecretData(namespace, name)
	if err != nil {
		return nil, err
	}
	projected := make([]k8s.DataEntry, len(entries))
	copy(projected, entries)
	sort.Slice(projected, func(i, j int) bool { return projected[i].Key < projected[j].Key })
	return projected, nil
}

// acceleratorSecretListItem is a closed wire DTO. The shared ordinary
// SecretMetadata contract intentionally remains richer for desktop callers.
type acceleratorSecretListItem struct {
	Metadata struct {
		Name              string      `json:"name"`
		Namespace         string      `json:"namespace"`
		UID               string      `json:"uid"`
		CreationTimestamp metav1.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Type     string `json:"type"`
	DataKeys int    `json:"dataKeys"`
}

func (a *App) acceleratorSecretYAML(namespace, name string) (string, error) {
	if a.k8sClient == nil {
		return "", fmt.Errorf("k8s client not initialized")
	}
	return a.k8sClient.GetSecretProjectedYaml(namespace, name)
}

func (a *App) acceleratorSecretsMetadata(requestID, namespace string, options k8s.SecretListOptions) ([]acceleratorSecretListItem, error) {
	items, err := a.listSecretsMetadataWithOptions(requestID, namespace, options)
	if err != nil {
		return nil, err
	}
	projected := make([]acceleratorSecretListItem, len(items))
	for i, item := range items {
		projected[i].Metadata.Name = item.Metadata.Name
		projected[i].Metadata.Namespace = item.Metadata.Namespace
		projected[i].Metadata.UID = item.Metadata.UID
		projected[i].Metadata.CreationTimestamp = item.Metadata.CreationTimestamp
		projected[i].Type = item.Type
		projected[i].DataKeys = item.DataKeys
	}
	return projected, nil
}
