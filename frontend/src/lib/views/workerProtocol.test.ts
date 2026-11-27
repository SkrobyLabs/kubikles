import { describe, expect, it } from 'vitest';
import { isPackedRefs, packRefs, unpackRefs } from './workerProtocol';

describe('packRefs', () => {
    it('round-trips refs through a string table and index buffer', () => {
        const refs = [
            { uid: 'u1', ns: 'a', name: 'x' },
            { uid: 'u2', ns: 'a', name: 'y' },
            { uid: 'u3', ns: '', name: 'x' },
        ];
        const p = packRefs(refs);
        expect(isPackedRefs(p)).toBe(true);
        expect(unpackRefs(p)).toEqual(refs);
        // namespaces and names are interned: 3 uids + a, '' , x, y
        expect(p.table.split('\u0000')).toHaveLength(7);
    });

    it('survives a transfer of the index buffer', () => {
        const refs = Array.from({ length: 50 }, (_, i) => ({ uid: `u${i}`, ns: `n${i % 3}`, name: `p${i}` }));
        const p = packRefs(refs);
        const copy = structuredClone(p, { transfer: [p.index] });
        expect(p.index.byteLength).toBe(0);
        expect(unpackRefs(copy)).toEqual(refs);
    });

    it('handles the empty list and a lone empty string', () => {
        expect(unpackRefs(packRefs([]))).toEqual([]);
        expect(unpackRefs(packRefs([{ uid: '', ns: '', name: '' }]))).toEqual([{ uid: '', ns: '', name: '' }]);
    });
});
