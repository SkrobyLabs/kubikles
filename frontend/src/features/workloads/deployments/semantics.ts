import type { SemanticsMap } from '~/utils/columnSemantics';
import { templateContainers, firstImage, imageLines, labelPairs } from '../workloadSemantics';
import { deploymentPodCount, deploymentPodsOf, podRefsText, type DeploymentPodMaps } from '../podDerivation';

/**
 * Sort/filter semantics of the Deployment list columns. The Pods column reads the pod maps the
 * list derives from the pod list; the other columns read the deployment alone.
 */
export function createDeploymentSemantics(maps: DeploymentPodMaps) {
    return {
        name: { getValue: (item: any) => item.metadata?.name },
        namespace: { getValue: (item: any) => item.metadata?.namespace },
        pods: { getValue: (item: any) => deploymentPodCount(maps.podCountsByUid, item), filterable: false },
        ready: { getValue: (item: any) => item.status?.readyReplicas || 0 },
        age: { getValue: (item: any) => item.metadata?.creationTimestamp },
        strategy: { getValue: (item: any) => item.spec?.strategy?.type || 'RollingUpdate' },
        replicas: { getValue: (item: any) => item.spec?.replicas ?? 1 },
        updatedReplicas: { getValue: (item: any) => item.status?.updatedReplicas ?? 0 },
        availableReplicas: { getValue: (item: any) => item.status?.availableReplicas ?? 0 },
        image: { getValue: (item: any) => firstImage(templateContainers(item)) },
        selector: { getValue: (item: any) => labelPairs(item.spec?.selector?.matchLabels) },
        /** Not a column of the list: the Ready cell's text, ready/total. */
        readyText: { getValue: (item: any) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, filterable: false },
        /** Not a column of the list: every image, one per line, for the Image cell's count and tooltip. */
        imageList: { getValue: (item: any) => imageLines(templateContainers(item)), filterable: false },
        /** Not a column of the list: the squares of the Pods cell. */
        podRefs: { getValue: (item: any) => podRefsText(deploymentPodsOf(maps, item)), filterable: false },
    } satisfies SemanticsMap;
}

/** The semantics with no pods known (the state of the list before the pod list arrives). */
export const deploymentSemantics = createDeploymentSemantics({ podsByDeployment: new Map(), podCountsByUid: new Map() });
