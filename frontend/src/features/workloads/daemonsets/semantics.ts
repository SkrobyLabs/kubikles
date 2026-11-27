import type { SemanticsMap } from '~/utils/columnSemantics';
import { templateContainers, firstImage, imageLines, labelPairs } from '../workloadSemantics';

/** Sort/filter semantics of the DaemonSet list columns. */
export const daemonSetSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    desired: { getValue: (item: any) => item.status?.desiredNumberScheduled || 0 },
    current: { getValue: (item: any) => item.status?.currentNumberScheduled || 0 },
    ready: { getValue: (item: any) => item.status?.numberReady || 0 },
    available: { getValue: (item: any) => item.status?.numberAvailable || 0 },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
    upToDate: { getValue: (item: any) => item.status?.updatedNumberScheduled || 0 },
    updateStrategy: { getValue: (item: any) => item.spec?.updateStrategy?.type || 'RollingUpdate' },
    image: { getValue: (item: any) => firstImage(templateContainers(item)) },
    selector: { getValue: (item: any) => labelPairs(item.spec?.selector?.matchLabels) },
    nodeSelector: { getValue: (item: any) => labelPairs(item.spec?.template?.spec?.nodeSelector) },
    /** Not a column of the list: every image, one per line, for the Image cell's count and tooltip. */
    imageList: { getValue: (item: any) => imageLines(templateContainers(item)), filterable: false },
} satisfies SemanticsMap;
