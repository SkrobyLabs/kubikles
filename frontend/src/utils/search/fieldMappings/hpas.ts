/**
 * HorizontalPodAutoscaler Field Mappings
 *
 * HPA-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const hpaFields = {
    ...commonFields,

    target: {
        extractor: (item) => {
            const ref = item.spec?.scaleTargetRef;
            if (!ref) return '';
            return `${ref.kind}/${ref.name}`;
        },
        aliases: ['reference', 'ref', 'scaletarget']
    },

    targetkind: {
        extractor: (item) => item.spec?.scaleTargetRef?.kind || '',
        aliases: ['kind', 'refkind']
    },

    targetname: {
        extractor: (item) => item.spec?.scaleTargetRef?.name || '',
        aliases: ['refname']
    },

    minreplicas: {
        extractor: (item) => String(item.spec?.minReplicas ?? 1),
        aliases: ['min']
    },

    maxreplicas: {
        extractor: (item) => String(item.spec?.maxReplicas || ''),
        aliases: ['max']
    },

    currentreplicas: {
        extractor: (item) => String(item.status?.currentReplicas ?? ''),
        aliases: ['current', 'replicas']
    },

    desiredreplicas: {
        extractor: (item) => String(item.status?.desiredReplicas ?? ''),
        aliases: ['desired']
    },

    metrics: {
        extractor: (item) => {
            const metrics = item.spec?.metrics || [];
            return metrics.map(m => m.type).join(' ');
        },
        aliases: ['metrictype']
    }
};
