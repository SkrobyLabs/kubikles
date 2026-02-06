/**
 * Endpoints Field Mappings
 *
 * Endpoints-specific fields for advanced search filtering.
 */

import { commonFields } from './common';

export const endpointsFields = {
    ...commonFields,

    subsets: {
        extractor: (item: any) => String(item.subsets?.length || 0),
        aliases: ['subsetcount']
    },

    addresses: {
        extractor: (item: any) => {
            const addresses: string[] = [];
            (item.subsets || []).forEach((subset: any) => {
                (subset.addresses || []).forEach((addr: any) => addresses.push(addr.ip));
            });
            return addresses.join(' ');
        },
        aliases: ['ips', 'ip']
    },

    ready: {
        extractor: (item: any) => {
            let count = 0;
            (item.subsets || []).forEach((subset: any) => {
                count += (subset.addresses || []).length;
            });
            return String(count);
        },
        aliases: ['readycount']
    },

    notready: {
        extractor: (item: any) => {
            let count = 0;
            (item.subsets || []).forEach((subset: any) => {
                count += (subset.notReadyAddresses || []).length;
            });
            return String(count);
        },
        aliases: ['notreadycount']
    },

    ports: {
        extractor: (item: any) => {
            const ports = new Set<any>();
            (item.subsets || []).forEach((subset: any) => {
                (subset.ports || []).forEach((port: any) => {
                    ports.add(`${port.port}/${port.protocol || 'TCP'}`);
                });
            });
            return Array.from(ports).join(' ');
        },
        aliases: ['port']
    },

    targetref: {
        extractor: (item: any) => {
            const refs: string[] = [];
            (item.subsets || []).forEach((subset: any) => {
                (subset.addresses || []).forEach((addr: any) => {
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
