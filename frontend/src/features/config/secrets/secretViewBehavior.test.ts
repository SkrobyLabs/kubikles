import { describe, expect, it } from 'vitest';
import { filterSecretsForView, HELM_RELEASE_SECRET_TYPE } from './secretViewBehavior';
import { optimizeNamespaceQuery } from '~/hooks/useNamespaceOptimization';
import { createNamespacedResourceEventHandler } from '~/hooks/useResourceEventHandler';

describe('filterSecretsForView', () => {
    it('preserves Direct Secret visible behavior', () => {
        const secrets = [{ type: 'Opaque' }, { type: HELM_RELEASE_SECRET_TYPE }];
        expect(filterSecretsForView(secrets, true)).toEqual([secrets[0]]);
        expect(filterSecretsForView(secrets, false)).toBe(secrets);
    });

    it('does not read or serialize Secret data', () => {
        const guarded = new Proxy({ type: 'Opaque' }, {
            get(target, key, receiver) {
                if (key === 'data') throw new Error('data accessed');
                return Reflect.get(target, key, receiver);
            },
            ownKeys() { throw new Error('serialized'); },
        });
        expect(filterSecretsForView([guarded], true)).toEqual([guarded]);
        expect(filterSecretsForView([guarded], false)).toBeInstanceOf(Array);
    });

    it('keeps seven visible rows namespaced and never needs data', () => {
        const rows = [
            ['empty', 'baseline-a', 'Opaque', 0], ['text', 'baseline-a', 'Opaque', 2], ['binary', 'baseline-a', 'Opaque', 1],
            ['helm', 'baseline-a', HELM_RELEASE_SECRET_TYPE, 1], ['churn', 'baseline-a', 'Opaque', 1], ['yaml', 'baseline-b', 'Opaque', 2], ['large', 'baseline-b', 'Opaque', 1],
        ].map(([name, namespace, type, dataKeys], i) => ({ metadata: { name: String(name), namespace: String(namespace), uid: `uid-${i}`, creationTimestamp: '2026-01-02T03:04:05Z' }, type: String(type), dataKeys: Number(dataKeys) }));
        expect(filterSecretsForView(rows, true)).toHaveLength(6);
        expect(filterSecretsForView(rows, false)).toHaveLength(7);
        expect(optimizeNamespaceQuery(['baseline-a', 'baseline-b'], ['baseline-a', 'baseline-b'])).toBe('');
        expect(optimizeNamespaceQuery(['baseline-a'], ['baseline-a', 'baseline-b'])).toEqual(['baseline-a']);
        const unsafeA = { ...rows[0], metadata: { ...rows[0].metadata, annotations: { marker: 'a' } }, data: { first: 'a' } };
        const unsafeB = { ...rows[0], metadata: { ...rows[0].metadata, annotations: { marker: 'b' } }, data: { second: 'b' } };
        const visible = ({ metadata, type, dataKeys }: any) => ({ name: metadata.name, namespace: metadata.namespace, uid: metadata.uid, creationTimestamp: metadata.creationTimestamp, type, dataKeys });
        expect(visible(unsafeA)).toEqual(visible(unsafeB));
        let state = new Map<string, any>();
        const handler = createNamespacedResourceEventHandler((update: any) => { state = update(state); }, ['baseline-a']);
        for (let i = 0; i < 33; i++) handler({ type: i === 0 ? 'ADDED' : 'MODIFIED', resource: rows[4], namespace: 'baseline-a' } as any);
        handler({ type: 'DELETED', resource: rows[4], namespace: 'baseline-a' } as any);
        handler({ type: 'ADDED', resource: rows[5], namespace: 'baseline-b' } as any);
        expect([...state.values()]).toEqual([]);
    });
});
