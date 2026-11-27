import { describe, expect, it } from 'vitest';
import { createFilter } from './filterEngine';
import { getAvailableFieldNames, getFieldByName, getFieldsForResource, getFieldsMetadata } from './fieldMappings';

// Characterization of alias resolution through the filter engine. The alias lookup
// inside getFieldByName is a candidate for hoisting/caching, so these tests pin the
// resolution order (direct name first, then first registry entry owning the alias),
// case-insensitivity and the fallback for unknown resource types.

const pod = (name: string, extra: any = {}) => ({
    metadata: { name, namespace: 'default', uid: `uid-${name}`, labels: {}, annotations: {}, ...extra.metadata },
    spec: { nodeName: '', ...extra.spec },
    status: { phase: 'Running', podIP: '', hostIP: '', containerStatuses: [], ...extra.status },
});

const pods = [
    pod('web-1', {
        metadata: { namespace: 'shop', labels: { app: 'web', tier: 'front' }, annotations: { owner: 'team-a' } },
        spec: { nodeName: 'node-a' },
        status: { podIP: '10.1.0.1', hostIP: '192.168.0.1', containerStatuses: [{ name: 'c', restartCount: 3 }] },
    }),
    pod('web-2', {
        metadata: { namespace: 'shop', labels: { app: 'web' } },
        spec: { nodeName: 'node-b' },
        status: { phase: 'Pending', podIP: '10.1.0.2' },
    }),
    pod('db-0', {
        metadata: { namespace: 'data', labels: { app: 'db' }, annotations: { owner: 'team-b' } },
        spec: { nodeName: 'node-a' },
        status: { podIP: '10.2.0.1', containerStatuses: [{ name: 'c', restartCount: 0 }] },
    }),
];

const names = (query: string, items: any[] = pods, type = 'pods') =>
    items.filter(createFilter(type, query)).map(p => p.metadata.name);

describe('field alias resolution in createFilter', () => {
    it.each([
        ['name:web', ['web-1', 'web-2']],
        ['n:web', ['web-1', 'web-2']],
        ['namespace:data', ['db-0']],
        ['ns:data', ['db-0']],
        ['labels:tier=front', ['web-1']],
        ['label:tier=front', ['web-1']],
        ['l:app=db', ['db-0']],
        ['annotations:owner=team-b', ['db-0']],
        ['annotation:owner=team-a', ['web-1']],
        ['a:team-', ['web-1', 'db-0']],
        ['nodename:node-a', ['web-1', 'db-0']],
        ['node:node-b', ['web-2']],
        ['status:Pending', ['web-2']],
        ['phase:Pending', ['web-2']],
        ['s:Pending', ['web-2']],
        ['ip:10.1.', ['web-1', 'web-2']],
        ['podip:10.2.', ['db-0']],
        ['hostip:192.168', ['web-1']],
        ['restarts:3', ['web-1']],
        ['restart:3', ['web-1']],
    ])('resolves %s', (query, expected) => {
        expect(names(query)).toEqual(expected);
    });

    it('treats field names and aliases case-insensitively', () => {
        expect(names('NS:data')).toEqual(['db-0']);
        expect(names('NodeName:node-b')).toEqual(['web-2']);
        expect(names('PHASE:pending')).toEqual(['web-2']);
    });

    it('matches nothing for an unknown field, even with an otherwise matching value', () => {
        expect(names('nosuchfield:web')).toEqual([]);
        // An unknown field in one AND-ed condition rejects the whole group.
        expect(names('name:web nosuchfield:web')).toEqual([]);
    });

    it('lets an OR group rescue an unknown-field group', () => {
        expect(names('nosuchfield:web OR name:db')).toEqual(['db-0']);
    });

    it('combines an alias condition with a regex on another alias', () => {
        expect(names('ns:shop s:/^pend/i')).toEqual(['web-2']);
    });

    it('yields the same selection whether a field is addressed by name or alias', () => {
        for (const [full, alias, value] of [
            ['namespace', 'ns', 'shop'], ['nodename', 'node', 'node-a'], ['status', 'phase', 'Running'],
            ['labels', 'l', 'app=web'], ['restarts', 'restart', '3'],
        ]) {
            expect(names(`${alias}:${value}`)).toEqual(names(`${full}:${value}`));
        }
    });

    it('reuses one compiled filter across many items without leaking regex state', () => {
        const filter = createFilter('pods', 'name:/web/g');
        // A /g regex reused across items must not skip matches via lastIndex.
        const results = [pods[0], pods[1], pods[0], pods[1]].map(filter);
        expect(results).toEqual([true, true, true, true]);
    });
});

describe('getFieldByName', () => {
    const resourceTypes = [
        'pods', 'deployments', 'statefulsets', 'daemonsets', 'replicasets', 'jobs', 'cronjobs', 'nodes', 'services',
        'secrets', 'configmaps', 'namespaces', 'events', 'serviceaccounts', 'roles', 'clusterroles', 'rolebindings',
        'clusterrolebindings', 'networkpolicies', 'hpas', 'pdbs', 'resourcequotas', 'limitranges', 'endpoints',
        'validatingwebhookconfigurations', 'mutatingwebhookconfigurations', 'priorityclasses', 'leases', 'csidrivers',
        'csinodes',
    ];

    it('prefers a direct field name, then the first registry entry that owns the alias', () => {
        for (const type of resourceTypes) {
            const fields = getFieldsForResource(type);
            const reference = (name: string) => {
                const lower = name.toLowerCase();
                if (fields[lower]) return fields[lower];
                for (const field of Object.values(fields) as any[]) {
                    if (field.aliases?.includes(lower)) return field;
                }
                return null;
            };
            for (const name of getAvailableFieldNames(type)) {
                const lower = name.toLowerCase();
                // Aliases containing capitals (e.g. numDrivers) are only reachable via their lowercased form
                // if they are declared lowercase; pin whatever the registry resolves them to today.
                expect(getFieldByName(type, name), `${type}:${name}`).toBe(reference(name));
                expect(getFieldByName(type, lower), `${type}:${lower}`).toBe(reference(lower));
            }
        }
    });

    it('falls back to the common fields for an unknown resource type', () => {
        expect(getFieldByName('widgets', 'ns')).toBe(getFieldsForResource('widgets').namespace);
        expect(getFieldByName('widgets', 'status')).toBeNull();
        expect(getFieldsMetadata('widgets').map(f => f.name)).toEqual(['name', 'namespace', 'labels', 'annotations', 'uid']);
    });

    it('returns null for unknown names and is stable across repeated lookups', () => {
        expect(getFieldByName('pods', 'bogus')).toBeNull();
        const first = getFieldByName('pods', 'node');
        for (let i = 0; i < 100; i++) expect(getFieldByName('pods', 'node')).toBe(first);
        expect(first).toBe(getFieldsForResource('pods').nodename);
    });

    it('exposes every alias through the metadata used for autocomplete', () => {
        const meta = getFieldsMetadata('pods');
        const status = meta.find(f => f.name === 'status')!;
        expect(status.aliases).toEqual(expect.arrayContaining(['phase', 's']));
        expect(meta.find(f => f.name === 'uid')!.aliases).toEqual([]);
    });
});

describe('scale', () => {
    it('filters 5000 pods through an aliased query and an OR group deterministically', () => {
        const big = Array.from({ length: 5000 }, (_, i) =>
            pod(`pod-${i}`, {
                metadata: { namespace: `ns-${i % 10}`, labels: { shard: String(i % 7) } },
                spec: { nodeName: `node-${i % 5}` },
                status: { phase: i % 3 === 0 ? 'Pending' : 'Running' },
            }));
        const expected = big.filter((_, i) => i % 10 === 3 && i % 5 === 3 && i % 3 !== 0).map(p => p.metadata.name);
        expect(names('ns:ns-3 node:node-3 phase:Running', big)).toEqual(expected);
        const either = names('ns:ns-3 OR node:node-4', big);
        expect(either).toEqual(big.filter((_, i) => i % 10 === 3 || i % 5 === 4).map(p => p.metadata.name));
        expect(names('l:shard=6 s:Pending', big)).toEqual(
            big.filter((_, i) => i % 7 === 6 && i % 3 === 0).map(p => p.metadata.name));
    });
});
