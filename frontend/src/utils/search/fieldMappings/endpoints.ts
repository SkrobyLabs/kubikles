/**
 * Endpoints Field Mappings
 *
 * Endpoints-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const endpointsFields = {
    ...commonFields,

    subsets: {
        extractor: (item) => String(item.subsets?.length || 0),
        aliases: ['subsetcount']
    },

    addresses: {
        extractor: (item) => {
            const addresses = [];
            (item.subsets || []).forEach(subset => {
                (subset.addresses || []).forEach(addr => addresses.push(addr.ip));
            });
            return addresses.join(' ');
        },
        aliases: ['ips', 'ip']
    },

    ready: {
        extractor: (item) => {
            let count = 0;
            (item.subsets || []).forEach(subset => {
                count += (subset.addresses || []).length;
            });
            return String(count);
        },
        aliases: ['readycount']
    },

    notready: {
        extractor: (item) => {
            let count = 0;
            (item.subsets || []).forEach(subset => {
                count += (subset.notReadyAddresses || []).length;
            });
            return String(count);
        },
        aliases: ['notreadycount']
    },

    ports: {
        extractor: (item) => {
            const ports = new Set();
            (item.subsets || []).forEach(subset => {
                (subset.ports || []).forEach(port => {
                    ports.add(`${port.port}/${port.protocol || 'TCP'}`);
                });
            });
            return Array.from(ports).join(' ');
        },
        aliases: ['port']
    },

    targetref: {
        extractor: (item) => {
            const refs = [];
            (item.subsets || []).forEach(subset => {
                (subset.addresses || []).forEach(addr => {
                    if (addr.targetRef) {
                        refs.push(`${addr.targetRef.kind}/${addr.targetRef.name}`);
                    }
                });
            });
            return [...new Set(refs)].join(' ');
        },
        aliases: ['target', 'targets']
    }
};
