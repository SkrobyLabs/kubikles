# Operator actions

Custom-resource list menus and detail headers share the same action catalog and confirmation dialog. The backend registry owns resource matching, target discovery and mutations. Adding an operator requires a provider registration; the UI contains no operator-name conditions.

## Behavior

- Every execution retains its originating Kubernetes context and revalidates source and target identities.
- All-target previews reject changes to their target set; selected-target previews accept only the selected, still-valid identities.
- Existing-resource mutations use UID and resourceVersion tests in one JSON patch. Multi-resource actions report results individually; they are not Kubernetes-wide transactions.
- Request acceptance is distinct from operator completion. Status and events remain the source of completion information.
- Backup creation uses stable request IDs and rejects changes to the previewed configuration. Replaying the same request reuses the same Backup.
- Workload restart requests use a stable pod-template annotation; replaying the same request does not trigger another rollout.
- Failed or uncertain requests require a fresh preview in the UI. A fresh preview is a new request; check prior outcomes before intentionally repeating it.

## Supported protocols

- [Strimzi rolling updates](https://strimzi.io/docs/operators/latest/deploying#assembly-rolling-updates-str): annotate Kafka StrimziPodSets or selected owned pods. Discover node pools by ownership and cluster association. Kafka v1beta2/v1 and core StrimziPodSet v1beta2 are supported; legacy StatefulSet installations are not. The entire Kafka scope includes broker and controller pools, excluding ZooKeeper and ancillary components.
- [cert-manager manual renewal implementation](https://github.com/cert-manager/cmctl/blob/main/pkg/renew/renew.go): set Issuing=True with reason ManuallyTriggered on certificates/status, preserving other conditions. Already-issuing certificates are reported as pending. Renewal does not guarantee a new ACME authorization challenge or force-reset a stuck, ongoing issuance. Issuer policies and rate limits still apply.
- [Prometheus Operator APIs](https://prometheus-operator.dev/docs/api-reference/api/) and [Grafana Agent API](https://grafana.com/docs/agent/latest/operator/api/): use spec.paused and spec.podMetadata.annotations. A rollout follows the managed workload update strategy, including any OnDelete behavior.
- [OpenTelemetry common API](https://github.com/open-telemetry/opentelemetry-operator/blob/main/apis/v1beta1/common.go): use the installed managementState values managed/unmanaged and podAnnotations. Collector sidecar mode cannot be restarted independently from its application.
- [Velero Schedule API](https://velero.io/docs/main/api-types/schedule/) and [backup builder](https://github.com/velero-io/velero/blob/main/pkg/builder/backup_builder.go): preserve template filters, storage settings, metadata overrides and optional owner references. A manual backup can run while its schedule is paused.
- [Velero validation predicate](https://github.com/velero-io/velero/blob/main/internal/storage/storagelocation.go): a missing lastValidationTime makes a storage location eligible at the next validation poll, without changing its frequency or fabricating a successful status.
- [Velero data movement](https://velero.io/docs/main/csi-snapshot-data-movement/): spec.cancel requests cancellation of a built-in data mover transfer. This does not cancel the entire parent backup/restore and may leave it incomplete.

## Installed CRD review

Read-only inventory of dev-ctrlp on 2026-09-08: 67 CRDs, with actions implemented for 16 installed resource types. Strimzi Kafka support is additional; its CRDs were not installed in this context. Every installed CRD is accounted for below. The table records the scope of this implementation, not a claim that an operator can never support more actions.

| CRD | Storage API version | Action or decision |
| --- | --- | --- |
| `alertmanagerconfigs.monitoring.coreos.com` | `v1alpha1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `alertmanagers.monitoring.coreos.com` | `v1` | Pause/resume reconciliation and request a rollout through the operator pod template. |
| `backuprepositories.velero.io` | `v1` | Repository maintenance is controller-managed; no verified one-shot maintenance request added. |
| `backups.velero.io` | `v1` | Run a terminal backup again as a new Backup request. |
| `backupstoragelocations.velero.io` | `v1` | Request another storage validation by clearing lastValidationTime. |
| `certificaterequests.cert-manager.io` | `v1` | Issuance artifact; renew the owning Certificate. No approval/denial or request deletion shortcut. |
| `certificates.cert-manager.io` | `v1` | Renew certificate through the guarded Issuing status condition. |
| `challenges.acme.cert-manager.io` | `v1` | ACME validation artifact; renewal is coordinated through Certificate, without resetting challenge status. |
| `clusterissuers.cert-manager.io` | `v1` | Renew all or selected referencing certificates across namespaces. |
| `configsyncstatuses.aks.clusterconfig.azure.com` | `v1beta1` | Platform-managed configuration or observed status; no verified user maintenance trigger. |
| `datadownloads.velero.io` | `v2alpha1` | Cancel an active built-in data-mover transfer. |
| `datauploads.velero.io` | `v2alpha1` | Cancel an active built-in data-mover transfer. |
| `deletebackuprequests.velero.io` | `v1` | Deletion request artifact; replay is not a useful maintenance action. |
| `downloadrequests.velero.io` | `v1` | Download request artifact; needs a dedicated result/download flow. |
| `environments.quix.io` | `v1` | Installed schema exposes environment identity and metadata; no verified manual maintenance trigger was available. |
| `extensionconfigs.aks.clusterconfig.azure.com` | `v1beta1` | Platform-managed configuration or observed status; no verified user maintenance trigger. |
| `grafanaagents.monitoring.grafana.com` | `v1alpha1` | Pause/resume reconciliation and request a rollout through the operator pod template. |
| `healthstates.azmon.container.insights` | `v1` | Platform-managed configuration or observed status; no verified user maintenance trigger. |
| `ingressroutes.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `ingressroutes.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `ingressroutetcps.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `ingressroutetcps.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `ingressrouteudps.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `ingressrouteudps.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `instrumentations.opentelemetry.io` | `v1alpha1` | Admission-injection configuration; restarting consuming applications requires a separate workload selection flow. |
| `integrations.monitoring.grafana.com` | `v1alpha1` | Agent discovery/configuration; maintenance acts on GrafanaAgent. |
| `issuers.cert-manager.io` | `v1` | Renew all or selected referencing certificates in the issuer namespace. |
| `logsinstances.monitoring.grafana.com` | `v1alpha1` | Agent discovery/configuration; maintenance acts on GrafanaAgent. |
| `m3dbclusters.operator.m3db.io` | `v1alpha1` | Installed schema lacks sufficient typed maintenance controls; no unverified database restart/reset added. |
| `metricsinstances.monitoring.grafana.com` | `v1alpha1` | Agent discovery/configuration; maintenance acts on GrafanaAgent. |
| `middlewares.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `middlewares.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `middlewaretcps.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `middlewaretcps.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `opampbridges.opentelemetry.io` | `v1alpha1` | Restart managed workloads through pod annotations. |
| `opentelemetrycollectors.opentelemetry.io` | `v1beta1` | Pause/resume reconciliation and request a rollout through the operator pod template. |
| `orders.acme.cert-manager.io` | `v1` | ACME issuance artifact; renewal is coordinated through Certificate, without resetting order status. |
| `podlogs.monitoring.grafana.com` | `v1alpha1` | Agent discovery/configuration; maintenance acts on GrafanaAgent. |
| `podmonitors.azmonitoring.coreos.com` | `v1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `podmonitors.monitoring.coreos.com` | `v1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `podvolumebackups.velero.io` | `v1` | Backup-owned transfer artifact; no supported spec cancellation control in the installed schema. |
| `podvolumerestores.velero.io` | `v1` | Restore-owned transfer artifact; no supported spec cancellation control in the installed schema. |
| `probes.monitoring.coreos.com` | `v1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `prometheusagents.monitoring.coreos.com` | `v1alpha1` | Pause/resume reconciliation and request a rollout through the operator pod template. |
| `prometheuses.monitoring.coreos.com` | `v1` | Pause/resume reconciliation and request a rollout through the operator pod template. |
| `prometheusrules.monitoring.coreos.com` | `v1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `restores.velero.io` | `v1` | Repeating a restore needs destination and existing-resource policy choices; no blind replay added. |
| `schedules.velero.io` | `v1` | Pause, resume, and create an idempotent one-off backup from the template. |
| `scrapeconfigs.monitoring.coreos.com` | `v1alpha1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `serverstatusrequests.velero.io` | `v1` | Diagnostic request artifact; needs a dedicated server-status view. |
| `serverstransports.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `serverstransports.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `serverstransporttcps.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `servicemonitors.azmonitoring.coreos.com` | `v1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `servicemonitors.monitoring.coreos.com` | `v1` | Monitoring discovery/rule configuration; maintenance acts on the owning monitoring workload. |
| `targetallocators.opentelemetry.io` | `v1alpha1` | Pause/resume reconciliation and request a rollout through the operator pod template. |
| `thanosrulers.monitoring.coreos.com` | `v1` | Pause/resume reconciliation and request a rollout through the operator pod template. |
| `tlsoptions.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `tlsoptions.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `tlsstores.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `tlsstores.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `traefikservices.traefik.containo.us` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `traefikservices.traefik.io` | `v1alpha1` | Routing/TLS configuration; changes are reconciled automatically. No verified pause/reload trigger. |
| `volumesnapshotclasses.snapshot.storage.k8s.io` | `v1` | Snapshot policy configuration, with no per-instance reconciliation trigger. |
| `volumesnapshotcontents.snapshot.storage.k8s.io` | `v1` | Storage-controller binding and lifecycle object; no manual status reset. |
| `volumesnapshotlocations.velero.io` | `v1` | Provider configuration; validation occurs in backup operations, with no verified independent validation trigger. |
| `volumesnapshots.snapshot.storage.k8s.io` | `v1` | A new snapshot or restore needs source PVC, class and retention choices; no blind recreation. |

## Verification

Provider and executor tests use fake Kubernetes clients. They cover ownership, paused and terminal states, identity drift, partial failures, certificate status preservation, spec preservation and backup request idempotency. Frontend interaction tests cover scopes, explicit selections, stale contexts, duplicate submissions and uncertain failures.

Optional read-only integration check:

```sh
KUBIKLES_ACTION_PREVIEW_CONTEXT=<context> go test ./pkg/k8s -run TestResourceActionLivePreviews -v
```

This check only discovers instances and prepares action previews. It never executes actions. The dev-ctrlp run found 16 registered resource types and successfully prepared 11 previews on existing eligible instances. Other registered types had no instances, or the sampled instance did not meet action prerequisites. No live mutation or operator completion was tested.
