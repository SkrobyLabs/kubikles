import type { SemanticsMap } from '~/utils/columnSemantics';
import { getController, controllerRefText } from '../workloadSemantics';

/** Sort/filter semantics of the ReplicaSet list columns. */
export const replicaSetSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    desired: { getValue: (item: any) => item.spec?.replicas || 0 },
    current: { getValue: (item: any) => item.status?.replicas || 0 },
    ready: { getValue: (item: any) => item.status?.readyReplicas || 0 },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
    controlledBy: { getValue: (item: any) => getController(item)?.kind || '' },
    /** Not a column of the list: the controller reference the "Controlled By" link is built from. */
    controllerRef: { getValue: controllerRefText, filterable: false },
} satisfies SemanticsMap;
