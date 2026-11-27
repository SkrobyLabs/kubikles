import { describe, it, expect } from 'vitest';
import { extractControllerOwnerFromYaml } from './k8s-helpers';

// Reference copy of the previous regex implementation, kept to prove the
// linear-time rewrite is behaviour-identical.
function reference(yamlContent: string) {
    if (!yamlContent) return null;
    const m = yamlContent.match(/ownerReferences:\s*\n((?:\s+-[\s\S]*?(?=\n\S|\n\s*$))+)/);
    if (!m) return null;
    const entries = m[1].split(/\n\s+-\s+/).filter(Boolean);
    for (const entry of entries) {
        if (entry.includes('controller: true') || entry.includes('controller:true')) {
            const k = entry.match(/kind:\s*(\S+)/);
            const n = entry.match(/name:\s*(\S+)/);
            const u = entry.match(/uid:\s*(\S+)/);
            if (k && n) return { kind: k[1], name: n[1], uid: u ? u[1] : null };
        }
    }
    return null;
}

function rng(seed: number) {
    return () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 2 ** 32;
}

describe('extractControllerOwnerFromYaml equivalence', () => {
    it('matches the reference on generated documents', () => {
        const r = rng(42);
        const pick = <T,>(a: T[]) => a[Math.floor(r() * a.length)];
        const pieces = [
            'metadata:\n', '  ownerReferences:\n', '  - apiVersion: apps/v1\n', '    controller: true\n',
            '    kind: ReplicaSet\n', '    name: web-1\n', '    uid: abc\n', '  - kind: Job\n', '    name: j\n',
            '  uid: self\n', 'spec:\n', '\n', '   \n', 'ownerReferences:\n', '- kind: X\n', '  name: y\n',
            '    f:ownerReferences: {}\n', '  name: pod\n', '\t- kind: T\n', 'status: {}', '  - controller:true\n',
        ];
        for (let i = 0; i < 4000; i++) {
            let doc = '';
            const n = 1 + Math.floor(r() * 12);
            for (let j = 0; j < n; j++) doc += pick(pieces);
            expect(extractControllerOwnerFromYaml(doc)).toEqual(reference(doc));
        }
    });

    it('stays fast on a large document', () => {
        let doc = 'metadata:\n  managedFields:\n';
        for (let i = 0; i < 20000; i++) doc += `  - manager: m${i}\n    fieldsV1:\n      f:ownerReferences: {}\n`;
        doc += '  ownerReferences:\n  - kind: ReplicaSet\n    name: rs\n    controller: true\nspec: {}\n';
        const t = performance.now();
        const out = extractControllerOwnerFromYaml(doc);
        expect(out).toMatchObject({ kind: 'ReplicaSet', name: 'rs' });
        expect(performance.now() - t).toBeLessThan(500);
    });
});
