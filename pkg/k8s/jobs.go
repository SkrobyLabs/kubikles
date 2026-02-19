package k8s

import (
	"context"
	"fmt"
	"log"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

func (c *Client) ListJobs(contextName, namespace string) ([]batchv1.Job, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListJobsWithContext(ctx, contextName, namespace)
}

// ListJobsWithContext lists jobs with cancellation support and pagination.
func (c *Client) ListJobsWithContext(ctx context.Context, contextName, namespace string, onProgress ...func(loaded, total int)) ([]batchv1.Job, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "jobs", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]batchv1.Job, string, *int64, error) {
		list, err := cs.BatchV1().Jobs(namespace).List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, progressFn)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) GetJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	job, err := cs.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(job)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var job batchv1.Job
	if err := yaml.Unmarshal([]byte(yamlContent), &job); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().Jobs(namespace).Update(ctx, &job, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteJob(contextName, namespace, name string) error {
	log.Printf("Deleting job: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// CronJob operations
func (c *Client) ListCronJobs(contextName, namespace string) ([]batchv1.CronJob, error) {
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return c.ListCronJobsWithContext(ctx, contextName, namespace)
}

// ListCronJobsWithContext lists cronjobs with cancellation support and pagination.
func (c *Client) ListCronJobsWithContext(ctx context.Context, contextName, namespace string, onProgress ...func(loaded, total int)) ([]batchv1.CronJob, error) {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	var progressFn func(loaded, total int)
	if len(onProgress) > 0 {
		progressFn = onProgress[0]
	}
	result, err := paginatedList(ctx, "cronjobs", defaultPageSize, func(ctx context.Context, opts metav1.ListOptions) ([]batchv1.CronJob, string, *int64, error) {
		list, err := cs.BatchV1().CronJobs(namespace).List(ctx, opts)
		if err != nil {
			return nil, "", nil, err
		}
		return list.Items, list.Continue, list.RemainingItemCount, nil
	}, progressFn)
	if err != nil {
		if isCancelledError(err) {
			return nil, ErrRequestCancelled
		}
		return nil, err
	}
	return result, nil
}

func (c *Client) GetCronJobYaml(namespace, name string) (string, error) {
	cs, err := c.getClientset()
	if err != nil {
		return "", err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(cronJob)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (c *Client) UpdateCronJobYaml(namespace, name, yamlContent string) error {
	cs, err := c.getClientset()
	if err != nil {
		return err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	var cronJob batchv1.CronJob
	if err := yaml.Unmarshal([]byte(yamlContent), &cronJob); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	_, err = cs.BatchV1().CronJobs(namespace).Update(ctx, &cronJob, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteCronJob(contextName, namespace, name string) error {
	log.Printf("Deleting cronjob: context=%s, ns=%s, name=%s", contextName, namespace, name)
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return cs.BatchV1().CronJobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) TriggerCronJob(contextName, namespace, cronJobName string) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Get the CronJob to use as template
	cronJob, err := cs.BatchV1().CronJobs(namespace).Get(ctx, cronJobName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get cronjob: %w", err)
	}

	// Create a Job from the CronJob spec
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJobName + "-manual-",
			Namespace:    namespace,
			Annotations: map[string]string{
				"cronjob.kubernetes.io/instantiate": "manual",
			},
		},
		Spec: cronJob.Spec.JobTemplate.Spec,
	}

	_, err = cs.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

func (c *Client) SuspendCronJob(contextName, namespace, name string, suspend bool) error {
	cs, err := c.getClientForContext(contextName)
	if err != nil {
		return fmt.Errorf("failed to get client for context %s: %w", contextName, err)
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()

	// Use JSON patch to update only the suspend field
	patchData := fmt.Sprintf(`{"spec":{"suspend":%t}}`, suspend)

	result, err := cs.BatchV1().CronJobs(namespace).Patch(
		ctx,
		name,
		types.MergePatchType,
		[]byte(patchData),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch cronjob: %w", err)
	}

	_ = result
	return nil
}

// PersistentVolumeClaim operations
