import type { SemanticsMap } from '~/utils/columnSemantics';
import { templateContainers, firstImage, imageLines } from '../workloadSemantics';
import { podRefsText } from '../podDerivation';

/**
 * Sort/filter semantics of the StatefulSet list columns. `podsOf` returns the pods a stateful set
 * selects (getDeploymentPods over the pod list in the list component).
 */
export function createStatefulSetSemantics(podsOf: (item: any) => any[]) {
    return {
        name: { getValue: (item: any) => item.metadata?.name },
        namespace: { getValue: (item: any) => item.metadata?.namespace },
        pods: { getValue: (item: any) => podsOf(item).length, filterable: false },
        ready: { getValue: (item: any) => item.status?.readyReplicas || 0 },
        age: { getValue: (item: any) => item.metadata?.creationTimestamp },
        replicas: { getValue: (item: any) => item.spec?.replicas ?? 1 },
        serviceName: { getValue: (item: any) => item.spec?.serviceName || '' },
        updateStrategy: { getValue: (item: any) => item.spec?.updateStrategy?.type || 'RollingUpdate' },
        image: { getValue: (item: any) => firstImage(templateContainers(item)) },
        volumeClaims: { getValue: (item: any) => (item.spec?.volumeClaimTemplates || []).length },
        /** Not a column of the list: the Ready cell's text, ready/total. */
        readyText: { getValue: (item: any) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, filterable: false },
        /** Not a column of the list: every image, one per line, for the Image cell's count and tooltip. */
        imageList: { getValue: (item: any) => imageLines(templateContainers(item)), filterable: false },
        /** Not a column of the list: the squares of the Pods cell. */
        podRefs: { getValue: (item: any) => podRefsText(podsOf(item)), filterable: false },
    } satisfies SemanticsMap;
}

/** The semantics with no pods known (the state of the list before the pod list arrives). */
export const statefulSetSemantics = createStatefulSetSemantics(() => []);
