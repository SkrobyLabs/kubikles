// The pod-derived values of the workload lists. The legacy deployment and stateful set lists pair a
// workload with the pods in its namespace whose labels contain every spec.selector.matchLabels
// entry (matchExpressions are ignored, a selector without matchLabels selects nothing). The List
// components and the golden-parity generator both import these, so the Go derived engine is
// proven against the code that really runs.
import { getEffectivePodStatus } from '~/utils/k8s-helpers';

export interface DeploymentPodMaps {
    /** Pods per `namespace/name`; present only for deployments that have matchLabels. */
    podsByDeployment: Map<string, any[]>;
    /** Pod count per deployment uid. */
    podCountsByUid: Map<any, number>;
}

/** Deployment -> pods mapping and counts (O(n*m); the pod list is filtered once per deployment). */
export function deploymentPodMaps(deployments: any[] | undefined, allPods: any[] | undefined): DeploymentPodMaps {
    const podsMap = new Map<string, any[]>();
    const countsMap = new Map<any, number>();
    if (!deployments || !allPods) return { podsByDeployment: podsMap, podCountsByUid: countsMap };

    for (const deployment of deployments) {
        const selector = deployment.spec?.selector?.matchLabels;
        if (!selector) continue;

        const deploymentKey = `${deployment.metadata?.namespace}/${deployment.metadata?.name}`;
        const matchingPods = allPods.filter((pod: any) => {
            if (pod.metadata.namespace !== deployment.metadata.namespace) return false;
            for (const [key, value] of Object.entries(selector)) {
                if (pod.metadata.labels?.[key] !== value) return false;
            }
            return true;
        });
        podsMap.set(deploymentKey, matchingPods);
        // Pre-compute count by UID for O(1) sort lookups
        countsMap.set(deployment.metadata?.uid, matchingPods.length);
    }
    return { podsByDeployment: podsMap, podCountsByUid: countsMap };
}

/** The deployment list's Pods sort value. */
export const deploymentPodCount = (counts: Map<any, number>, item: any): number => counts.get(item.metadata?.uid) ?? 0;

/** The pods of a deployment from the precomputed map. */
export const deploymentPodsOf = (maps: DeploymentPodMaps, item: any): any[] =>
    maps.podsByDeployment.get(`${item.metadata?.namespace}/${item.metadata?.name}`) || [];

/**
 * The squares of the Pods cell as one string: JSON of [name, uid, effective status] per pod, '' for
 * none. Not a column of the legacy list (it draws the squares from the pods themselves); the view
 * path carries it as a display-only cell.
 */
export const podRefsText = (pods: any[]): string =>
    pods.length === 0 ? '' : JSON.stringify(pods.map((p: any) => [p.metadata.name, p.metadata.uid, getEffectivePodStatus(p)]));

/** Placeholder squares while pods load: min(replicas ?? 1, 5), plus whether an ellipsis follows. */
export function placeholderSquares(replicas: number | undefined | null): { count: number; more: boolean } {
    const count = replicas ?? 1;
    return { count: Math.min(count, 5), more: count > 5 };
}
