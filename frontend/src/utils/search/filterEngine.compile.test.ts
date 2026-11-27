import { describe, expect, it, vi } from 'vitest';

const lookups = vi.hoisted(() => ({ calls: 0 }));
vi.mock('./fieldMappings', async () => {
    const actual: any = await vi.importActual('./fieldMappings');
    return { ...actual, getFieldByName: (...args: any[]) => { lookups.calls++; return actual.getFieldByName(...args); } };
});

import { createFilter } from './filterEngine';
import { getFieldByName } from './fieldMappings';

const pods = Array.from({ length: 500 }, (_, i) => ({
    metadata: { name: `pod-${i}`, namespace: i % 2 ? 'prod' : 'dev', uid: `u${i}` },
    status: { phase: i % 3 ? 'Running' : 'Pending' },
}));

describe('createFilter compilation', () => {
    it('resolves each field once per filter, not once per item', () => {
        lookups.calls = 0;
        const filter = createFilter('pods', 'ns:prod status:Running OR name:/^pod-1/');
        expect(lookups.calls).toBe(3);
        const matched = pods.filter(filter);
        expect(lookups.calls).toBe(3);
        expect(matched.length).toBeGreaterThan(0);
    });

    it('keeps AND within a group and OR across groups', () => {
        const names = (q: string) => pods.filter(createFilter('pods', q)).map(p => p.metadata.name);
        expect(names('ns:prod name:pod-1')).toEqual(pods.filter(p => p.metadata.namespace === 'prod' && p.metadata.name.includes('pod-1')).map(p => p.metadata.name));
        expect(names('name:/^pod-10$/ OR name:/^pod-11$/')).toEqual(['pod-10', 'pod-11']);
    });

    it('unknown fields never match and plain text matches names case-insensitively', () => {
        expect(pods.filter(createFilter('pods', 'nonsense:abc'))).toHaveLength(0);
        expect(pods.filter(createFilter('pods', 'POD-49'))).toHaveLength(11);
        expect(pods.filter(createFilter('pods', ''))).toHaveLength(500);
    });
});

describe('getFieldByName lookup table', () => {
    it('does not rescan the registry per lookup', () => {
        getFieldByName('pods', 'ns');
        const entries = vi.spyOn(Object, 'entries');
        const values = vi.spyOn(Object, 'values');
        for (let i = 0; i < 1000; i++) getFieldByName('pods', i % 2 ? 'NS' : 'status');
        expect(entries).not.toHaveBeenCalled();
        expect(values).not.toHaveBeenCalled();
        entries.mockRestore();
        values.mockRestore();
    });

    it('resolves direct names before aliases, case-insensitively, and returns null for unknown names', () => {
        const direct = getFieldByName('pods', 'name');
        expect(getFieldByName('pods', 'NAME')).toBe(direct);
        expect(getFieldByName('pods', 'n')).toBeTruthy();
        expect(getFieldByName('pods', 'does-not-exist')).toBeNull();
        expect(getFieldByName('constructor', 'toString')).toBeNull();
    });
});
