import type { SemanticsMap } from '~/utils/columnSemantics';

export const hpaScaleTarget = (hpa: any) => {
    const ref = hpa.spec?.scaleTargetRef;
    if (!ref) return '-';
    return `${ref.kind}/${ref.name}`;
};

export const hpaMinMax = (hpa: any) => {
    const min = hpa.spec?.minReplicas ?? 1;
    const max = hpa.spec?.maxReplicas ?? '-';
    return `${min}/${max}`;
};

/** A number when the controller reported replicas, otherwise the placeholder dash. */
export const hpaReplicas = (hpa: any) => hpa.status?.currentReplicas ?? '-';

/** Sort/filter semantics of the HorizontalPodAutoscaler list columns. */
export const hpaSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    reference: { getValue: hpaScaleTarget },
    minmax: { getValue: hpaMinMax },
    replicas: { getValue: hpaReplicas },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
