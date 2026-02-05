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
        extractor: (item) => {
            const secrets = item.secrets || [];
            return secrets.map(s => s.name).join(' ');
        },
        aliases: ['secret']
    },

    secretcount: {
        extractor: (item) => String((item.secrets || []).length),
        aliases: ['secretscount', 'numsecrets']
    },

    imagepullsecrets: {
        extractor: (item) => {
            const secrets = item.imagePullSecrets || [];
            return secrets.map(s => s.name).join(' ');
        },
        aliases: ['pullsecrets', 'imagepull']
    },

    automount: {
        extractor: (item) => {
            const val = item.automountServiceAccountToken;
            return val === undefined ? 'true' : String(val);
        },
        aliases: ['automounttoken', 'automountserviceaccounttoken']
    }
};
