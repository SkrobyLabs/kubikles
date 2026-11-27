import type { SemanticsMap } from '~/utils/columnSemantics';

export const configMapKeysText = (item: any) => Object.keys(item.data || {}).join(', ');
export const configMapKeyCount = (item: any) => Object.keys(item.data || {}).length;
export const configMapBinaryKeyCount = (item: any) => Object.keys(item.binaryData || {}).length;
export const configMapSize = (item: any) =>
    JSON.stringify(item.data || {}).length + JSON.stringify(item.binaryData || {}).length;

/** Sort/filter semantics of the ConfigMap list columns that derive from the object itself. */
export const configMapSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    keys: { getValue: configMapKeysText },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
    keyCount: { getValue: configMapKeyCount },
    binaryKeys: { getValue: configMapBinaryKeyCount },
    size: { getValue: configMapSize },
} satisfies SemanticsMap;
