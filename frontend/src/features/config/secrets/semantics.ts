import type { SemanticsMap } from '~/utils/columnSemantics';

export const HELM_RELEASE_SECRET_TYPE = 'helm.sh/release.v1';

/** Sort/filter semantics of the Secret list columns that derive from the object itself. */
export const secretSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    type: { getValue: (item: any) => item.type },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
    // List items carry no data (metadata-only Table list), so these two only have values on
    // objects that arrived through a full-object watch event. The view path replaces them.
    keys: { getValue: (item: any) => Object.keys(item.data || {}).join(',') },
    size: {
        getValue: (item: any) => Object.values(item.data || {}).reduce((sum: any, v: any) => sum + (v?.length || 0), 0),
    },
} satisfies SemanticsMap;
