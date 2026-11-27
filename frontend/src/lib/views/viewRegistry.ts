// The table that states, for every list type, whether it is served by a Go view or by the legacy
// hook, and why not when it is not. viewRegistry.test.ts keeps it honest: a list that exists in
// the app but is neither migrated nor listed here with a reason fails the build, and so does a
// migrated type (a generated row schema) that is not marked as such.
//
// Keys are the store types the list hooks use (hooks/resources), plus the lists that have no
// store type (Helm releases and repositories, the CRD list). Custom resource instances are
// always served by a view; they have no static entry because their type is dynamic ("crd:...").

export type ListViewStatus =
    | { onViewPath: true }
    | { onViewPath: false; reason: string };

const migrated: ListViewStatus = { onViewPath: true };
const pending = (reason: string): ListViewStatus => ({ onViewPath: false, reason });

const PENDING_CRONJOB = 'Not migrated: the Next Run column is computed at render time from the cron schedule and the wall clock (a cron-parser run to now, shown as a duration, and used as a string sort and filter value), so no cell stored with the object can match it, and a Go cron parser would have to reproduce cron-parser bug for bug.';

export const LIST_VIEW_REGISTRY: Readonly<Record<string, ListViewStatus>> = {
    // Migrated: served by pkg/rows specs (see the generated row schemas).
    namespaces: migrated,
    configmaps: migrated,
    secrets: migrated,
    events: migrated,
    priorityclasses: migrated,
    ingressclasses: migrated,
    storageclasses: migrated,
    validatingwebhookconfigurations: migrated,
    mutatingwebhookconfigurations: migrated,
    csidrivers: migrated,
    csinodes: migrated,
    persistentvolumes: migrated,
    clusterroles: migrated,
    clusterrolebindings: migrated,
    hpas: migrated,
    pdbs: migrated,
    resourcequotas: migrated,
    limitranges: migrated,
    leases: migrated,
    networkpolicies: migrated,
    serviceaccounts: migrated,
    rolebindings: migrated,
    endpoints: migrated,
    endpointslices: migrated,
    services: migrated,
    ingresses: migrated,
    roles: migrated,
    replicasets: migrated,
    jobs: migrated,
    daemonsets: migrated,
    persistentvolumeclaims: migrated,
    nodes: migrated,
    pods: migrated,

    // Workloads: the Pods column is derived from the pod list inside the view (Spec.Derived).
    deployments: migrated,
    statefulsets: migrated,

    // Workloads, pending.
    cronjobs: pending(PENDING_CRONJOB),

    // No store type: these lists are not fed by the shared store.
    helmreleases: pending('Not a store-backed list: releases come from the Helm SDK, not from a Kubernetes list or watch.'),
    chartsources: pending('Not cluster data: the list shows locally configured chart repositories.'),
    crds: pending('Not migrated yet: CustomResourceDefinitions are listed through the dedicated CRD hook, not the store.'),
};

export function listViewStatus(type: string): ListViewStatus | undefined {
    return LIST_VIEW_REGISTRY[type];
}
