/**
 * ServiceAccount Field Mappings
 *
 * ServiceAccount-specific fields for advanced search filtering.
 * Extends common fields with ServiceAccount-specific extractors.
 */

import { commonFields } from './common';

export const serviceAccountFields = {
    ...commonFields,

    secrets: {
        extractor: (item: any) => {
            const secrets = item.secrets || [];
            return secrets.map((s: any) => s.name).join(' ');
        },
        aliases: ['secret']
    },

    secretcount: {
        extractor: (item: any) => String((item.secrets || []).length),
        aliases: ['secretscount', 'numsecrets']
    },

    imagepullsecrets: {
        extractor: (item: any) => {
            const secrets = item.imagePullSecrets || [];
            return secrets.map((s: any) => s.name).join(' ');
        },
        aliases: ['pullsecrets', 'imagepull']
    },

    automount: {
        extractor: (item: any) => {
            const val = item.automountServiceAccountToken;
            return val === undefined ? 'true' : String(val);
        },
        aliases: ['automounttoken', 'automountserviceaccounttoken']
    }
};
