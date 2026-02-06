/**
 * HorizontalPodAutoscaler Field Mappings
 *
 * HPA-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const hpaFields = {
    ...commonFields,

    target: {
        extractor: (item: any) => {
            const ref = item.spec?.scaleTargetRef;
            if (!ref) return '';
            return `${ref.kind}/${ref.name}`;
        },
        aliases: ['reference', 'ref', 'scaletarget']
    },

    targetkind: {
        extractor: (item: any) => item.spec?.scaleTargetRef?.kind || '',
        aliases: ['kind', 'refkind']
    },

    targetname: {
        extractor: (item: any) => item.spec?.scaleTargetRef?.name || '',
        aliases: ['refname']
    },

    minreplicas: {
        extractor: (item: any) => String(item.spec?.minReplicas ?? 1),
        aliases: ['min']
    },

    maxreplicas: {
        extractor: (item: any) => String(item.spec?.maxReplicas || ''),
        aliases: ['max']
    },

    currentreplicas: {
        extractor: (item: any) => String(item.status?.currentReplicas ?? ''),
        aliases: ['current', 'replicas']
    },

    desiredreplicas: {
        extractor: (item: any) => String(item.status?.desiredReplicas ?? ''),
        aliases: ['desired']
    },

    metrics: {
        extractor: (item: any) => {
            const metrics = item.spec?.metrics || [];
            return metrics.map((m: any) => m.type).join(' ');
        },
        aliases: ['metrictype']
    }
};
