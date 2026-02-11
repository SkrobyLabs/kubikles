package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListConfigMaps(namespace string) ([]v1.ConfigMap, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	log.Printf("[ListConfigMaps] getClientset took %v", time.Since(start))
	apiStart := time.Now()
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	log.Printf("[ListConfigMaps] API call took %v, returned %d items", time.Since(apiStart), len(cms.Items))
	if err != nil {
		return nil, err
	}
	return cms.Items, nil
}

// ListConfigMapsWithContext lists configmaps with cancellation support
func (c *Client) ListConfigMapsWithContext(ctx context.Context, namespace string) ([]v1.ConfigMap, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	getClientsetTime := time.Since(start)

	apiStart := time.Now()
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	apiTime := time.Since(apiStart)

	// Check context state
	ctxErr := ctx.Err()
	deadline, hasDeadline := ctx.Deadline()
	deadlineInfo := "no deadline"
	if hasDeadline {
		deadlineInfo = fmt.Sprintf("deadline in %v", time.Until(deadline))
	}

	log.Printf("[ListConfigMapsWithContext] getClientset=%v, API=%v, total=%v, ns=%q, items=%d, err=%v, ctxErr=%v, %s",
		getClientsetTime, apiTime, time.Since(start), namespace, len(cms.Items), err, ctxErr, deadlineInfo)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return cms.Items, nil
}

// ListConfigMapsForContext lists configmaps for a specific kubeconfig context
func (c *Client) ListConfigMapsForContext(contextName, namespace string) ([]v1.ConfigMap, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return cms.Items, nil
}

func (c *Client) ListSecrets(namespace string) ([]v1.Secret, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	log.Printf("[ListSecrets] getClientset took %v", time.Since(start))
	apiStart := time.Now()
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	log.Printf("[ListSecrets] API call took %v, returned %d items", time.Since(apiStart), len(secrets.Items))
	if err != nil {
		return nil, err
	}
	// Sanitize secrets? For now, we return them as is, UI should handle masking.
	return secrets.Items, nil
}

// ListSecretsWithContext lists secrets with cancellation support
func (c *Client) ListSecretsWithContext(ctx context.Context, namespace string) ([]v1.Secret, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	log.Printf("[ListSecretsWithContext] API call took %v, ns=%q, items=%d, err=%v", time.Since(start), namespace, len(secrets.Items), err)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return secrets.Items, nil
}

// ListSecretsForContext lists secrets for a specific kubeconfig context
func (c *Client) ListSecretsForContext(contextName, namespace string) ([]v1.Secret, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return secrets.Items, nil
}

// SecretListItem is a lightweight representation of a Secret for list views.
// It contains only the fields needed for display, avoiding transfer of actual secret data.
type SecretListItem struct {
	Metadata SecretMetadata `json:"metadata"`
	Type     string         `json:"type"`
	DataKeys int            `json:"dataKeys"` // Number of data keys, not the actual data
}

// SecretMetadata contains only the metadata fields needed for list display
type SecretMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	CreationTimestamp metav1.Time       `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

// ListSecretsMetadataWithContext lists secrets using metadata-only fetch for list views.
// This avoids transferring the actual secret data, significantly reducing response size.
func (c *Client) ListSecretsMetadataWithContext(ctx context.Context, namespace string) ([]SecretListItem, error) {
	start := time.Now()
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}

	// Build the request path
	var path string
	if namespace == "" {
		path = "/api/v1/secrets"
	} else {
		path = fmt.Sprintf("/api/v1/namespaces/%s/secrets", namespace)
	}

	// Use Table format with metadata-only objects
	// This returns column data (name, type, data count, age) plus minimal object metadata
	// without the actual secret data
	result := cs.CoreV1().RESTClient().Get().
		AbsPath(path).
		SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1").
		Do(ctx)

	if err := result.Error(); err != nil {
		log.Printf("[ListSecretsMetadata] API call failed after %v, ns=%q, err=%v", time.Since(start), namespace, err)
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}

	// Parse the Table response
	body, err := result.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var table metav1.Table
	if err := json.Unmarshal(body, &table); err != nil {
		return nil, fmt.Errorf("failed to parse table response: %w", err)
	}

	// Find column indices
	nameIdx, typeIdx, dataIdx := -1, -1, -1
	for i, col := range table.ColumnDefinitions {
		switch col.Name {
		case "Name":
			nameIdx = i
		case "Type":
			typeIdx = i
		case "Data":
			dataIdx = i
		}
	}

	// Convert rows to SecretListItem
	items := make([]SecretListItem, 0, len(table.Rows))
	for _, row := range table.Rows {
		item := SecretListItem{}

		// Extract cells
		if nameIdx >= 0 && nameIdx < len(row.Cells) {
			if name, ok := row.Cells[nameIdx].(string); ok {
				item.Metadata.Name = name
			}
		}
		if typeIdx >= 0 && typeIdx < len(row.Cells) {
			if t, ok := row.Cells[typeIdx].(string); ok {
				item.Type = t
			}
		}
		if dataIdx >= 0 && dataIdx < len(row.Cells) {
			// Data column contains count as number
			switch v := row.Cells[dataIdx].(type) {
			case float64:
				item.DataKeys = int(v)
			case int64:
				item.DataKeys = int(v)
			case int:
				item.DataKeys = v
			}
		}

		// Extract metadata from the embedded object
		if row.Object.Raw != nil {
			var partialMeta struct {
				Metadata struct {
					Name              string            `json:"name"`
					Namespace         string            `json:"namespace"`
					UID               string            `json:"uid"`
					CreationTimestamp metav1.Time       `json:"creationTimestamp"`
					Labels            map[string]string `json:"labels,omitempty"`
					Annotations       map[string]string `json:"annotations,omitempty"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(row.Object.Raw, &partialMeta); err == nil {
				item.Metadata.Name = partialMeta.Metadata.Name
				item.Metadata.Namespace = partialMeta.Metadata.Namespace
				item.Metadata.UID = partialMeta.Metadata.UID
				item.Metadata.CreationTimestamp = partialMeta.Metadata.CreationTimestamp
				item.Metadata.Labels = partialMeta.Metadata.Labels
				item.Metadata.Annotations = partialMeta.Metadata.Annotations
			}
		}

		items = append(items, item)
	}

	log.Printf("[ListSecretsMetadata] API call took %v, ns=%q, items=%d", time.Since(start), namespace, len(items))
	return items, nil
}

// ConfigMap YAML operations
func (c *Client) GetConfigMapYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	configMap, err := cs.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	configMap.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(configMap)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateConfigMapYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var configMap v1.ConfigMap
	if err := yaml.Unmarshal([]byte(yamlContent), &configMap); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().ConfigMaps(namespace).Update(ctx, &configMap, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteConfigMap(contextName, namespace, name string) error {
	log.Printf("Deleting configmap: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) GetConfigMapData(namespace, name string) (map[string]string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cm, err := cs.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range cm.Data {
		result[k] = v
	}
	return result, nil
}

// UpdateConfigMapData updates the configmap's data from a map of key -> value
func (c *Client) UpdateConfigMapData(namespace, name string, data map[string]string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cm, err := cs.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	cm.Data = data
	_, err = cs.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// Secret YAML operations
func (c *Client) GetSecretYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// Remove managed fields
	secret.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(secret)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateSecretYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var secret v1.Secret
	if err := yaml.Unmarshal([]byte(yamlContent), &secret); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(ctx, &secret, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteSecret(contextName, namespace, name string) error {
	log.Printf("Deleting secret: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// GetSecretData returns the secret's data as a map of key -> base64-encoded value
func (c *Client) GetSecretData(namespace, name string) (map[string]string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for k, v := range secret.Data {
		result[k] = string(v)
	}
	return result, nil
}

// UpdateSecretData updates the secret's data from a map of key -> value (values are raw strings, will be stored as bytes)
func (c *Client) UpdateSecretData(namespace, name string, data map[string]string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	secret.Data = make(map[string][]byte)
	for k, v := range data {
		secret.Data[k] = []byte(v)
	}
	_, err = cs.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}
