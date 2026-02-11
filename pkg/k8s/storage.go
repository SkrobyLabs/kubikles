package k8s

import (
	"context"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListPVCs(contextName, namespace string) ([]v1.PersistentVolumeClaim, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvcs.Items, nil
}

func (c *Client) ListPVCsWithContext(ctx context.Context, contextName, namespace string) ([]v1.PersistentVolumeClaim, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return pvcs.Items, nil
}

func (c *Client) GetPVCYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pvc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pvc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVCYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pvc v1.PersistentVolumeClaim
	if err := yaml.Unmarshal([]byte(yamlContent), &pvc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, &pvc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePVC(contextName, namespace, name string) error {
	log.Printf("Deleting PVC: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) ResizePVC(contextName, namespace, name, newSize string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Get current PVC
	pvc, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get PVC: %w", err)
	}

	// Parse and validate new size
	newQuantity, err := resource.ParseQuantity(newSize)
	if err != nil {
		return fmt.Errorf("invalid size format: %w", err)
	}

	// Check that new size is larger than current
	currentSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
	if newQuantity.Cmp(currentSize) <= 0 {
		return fmt.Errorf("new size must be larger than current size (%s)", currentSize.String())
	}

	// Update the storage request
	pvc.Spec.Resources.Requests[v1.ResourceStorage] = newQuantity

	_, err = cs.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to resize PVC: %w", err)
	}

	return nil
}

// PersistentVolume operations (cluster-scoped)
func (c *Client) ListPVs(contextName string) ([]v1.PersistentVolume, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pvs, err := cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvs.Items, nil
}

func (c *Client) ListPVsWithContext(ctx context.Context, contextName string) ([]v1.PersistentVolume, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	pvs, err := cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return pvs.Items, nil
}

func (c *Client) GetPVYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	pv, err := cs.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	pv.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(pv)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdatePVYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var pv v1.PersistentVolume
	if err := yaml.Unmarshal([]byte(yamlContent), &pv); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.CoreV1().PersistentVolumes().Update(ctx, &pv, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeletePV(contextName, name string) error {
	log.Printf("Deleting PV: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.CoreV1().PersistentVolumes().Delete(ctx, name, metav1.DeleteOptions{})
}

// StorageClass operations (cluster-scoped)
func (c *Client) GetStorageClass(contextName, name string) (*storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	sc, err := cs.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return sc, nil
}

func (c *Client) ListStorageClasses(contextName string) ([]storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	scs, err := cs.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return scs.Items, nil
}

func (c *Client) ListStorageClassesWithContext(ctx context.Context, contextName string) ([]storagev1.StorageClass, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	scs, err := cs.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return scs.Items, nil
}

func (c *Client) GetStorageClassYaml(name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	sc, err := cs.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	sc.ManagedFields = nil

	yamlBytes, err := yaml.Marshal(sc)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateStorageClassYaml(name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var sc storagev1.StorageClass
	if err := yaml.Unmarshal([]byte(yamlContent), &sc); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.StorageV1().StorageClasses().Update(ctx, &sc, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteStorageClass(contextName, name string) error {
	log.Printf("Deleting StorageClass: context=%s, name=%s", contextName, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.StorageV1().StorageClasses().Delete(ctx, name, metav1.DeleteOptions{})
}

// getApiExtensionsClientForContext returns an apiextensions clientset for a given context
