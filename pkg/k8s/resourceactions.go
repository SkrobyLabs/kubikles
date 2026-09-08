package k8s

import (
	"fmt"

	"kubikles/pkg/resourceactions"
	"kubikles/pkg/resourceactions/certmanager"
	"kubikles/pkg/resourceactions/grafana"
	"kubikles/pkg/resourceactions/monitoring"
	"kubikles/pkg/resourceactions/opentelemetry"
	"kubikles/pkg/resourceactions/strimzi"
	"kubikles/pkg/resourceactions/velero"
)

// Providers are registered here; CRD screens do not contain operator rules.
var operatorActions = resourceactions.New(
	strimzi.Provider{},
	certmanager.Provider{},
	certmanager.IssuerProvider{},
	certmanager.IssuerProvider{Cluster: true},
	monitoring.Prometheus(),
	monitoring.Alertmanager(),
	monitoring.PrometheusAgent(),
	monitoring.ThanosRuler(),
	grafana.Agent(),
	opentelemetry.Collector(),
	opentelemetry.TargetAllocator(),
	opentelemetry.OpAMPBridge(),
	velero.StorageProvider{},
	velero.ScheduleProvider{},
	velero.BackupProvider{},
	velero.TransferProvider{},
	velero.TransferProvider{Download: true},
)

func ResourceActions(group, resource string) []resourceactions.Action {
	return operatorActions.Actions(group, resource)
}
func (c *Client) PrepareResourceAction(ref resourceactions.ResourceRef, id, mode string) (resourceactions.Plan, error) {
	if ref.Context == "" {
		return resourceactions.Plan{}, fmt.Errorf("resource context is required")
	}
	client, err := c.getDynamicClientForContext(ref.Context)
	if err != nil {
		return resourceactions.Plan{}, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return operatorActions.Prepare(ctx, client, ref, id, mode)
}
func (c *Client) ExecuteResourceAction(plan resourceactions.Plan) ([]resourceactions.Result, error) {
	if plan.Source.Context == "" {
		return nil, fmt.Errorf("resource context is required")
	}
	client, err := c.getDynamicClientForContext(plan.Source.Context)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.contextWithTimeout()
	defer cancel()
	return operatorActions.Execute(ctx, client, plan)
}
