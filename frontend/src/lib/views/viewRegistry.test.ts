import { describe, expect, it } from 'vitest';
import { LIST_VIEW_REGISTRY, listViewStatus } from './viewRegistry';
import { migratedTypes, hasRowSchema } from './schema';

// Source files are read as text so the test sees every list the app really has, not a copy of
// the list kept in this file.
const hookSource = Object.values(import.meta.glob('../../hooks/resources/index.ts', { query: '?raw', import: 'default', eager: true }))[0] as string;
const listSources = import.meta.glob('../../features/**/*List.tsx', { query: '?raw', import: 'default', eager: true }) as Record<string, string>;

/** List ids a ResourceList is given that differ from the store type of its hook. */
const LIST_ID_TO_TYPE: Record<string, string> = { pvs: 'persistentvolumes', pvcs: 'persistentvolumeclaims' };

function hookTypes(): string[] {
    return [...hookSource.matchAll(/viewCapable(?:Namespaced|Cluster)\(\s*'([a-z]+)'/g)].map(m => m[1]);
}

function listIds(): string[] {
    const ids = new Set<string>();
    for (const src of Object.values(listSources)) {
        for (const m of src.matchAll(/resourceType="([a-z]+)"/g)) ids.add(LIST_ID_TO_TYPE[m[1]] ?? m[1]);
    }
    return [...ids];
}

describe('list view registry', () => {
    it('discovers the lists of the app', () => {
        // A guard against the discovery silently finding nothing.
        expect(hookTypes().length).toBeGreaterThan(30);
        expect(listIds().length).toBeGreaterThan(30);
    });

    it('states a view status for every list type the hooks define', () => {
        const missing = hookTypes().filter(t => !listViewStatus(t));
        expect(missing, `list types neither migrated nor listed with a reason: ${missing.join(', ')}`).toEqual([]);
    });

    it('states a view status for every list a ResourceList is rendered for', () => {
        const missing = listIds().filter(t => !listViewStatus(t));
        expect(missing, `lists neither migrated nor listed with a reason: ${missing.join(', ')}`).toEqual([]);
    });

    it('has no entry for a list that no longer exists', () => {
        const known = new Set([...hookTypes(), ...listIds()]);
        const stale = Object.keys(LIST_VIEW_REGISTRY).filter(t => !known.has(t));
        expect(stale).toEqual([]);
    });

    it('marks exactly the types with a generated row schema as on the view path', () => {
        const onPath = Object.entries(LIST_VIEW_REGISTRY).filter(([, s]) => s.onViewPath).map(([t]) => t).sort();
        expect(onPath).toEqual(migratedTypes().sort());
        for (const [type, status] of Object.entries(LIST_VIEW_REGISTRY)) {
            expect(hasRowSchema(type), type).toBe(status.onViewPath);
        }
    });

    it('gives every list that is not on the view path a reason', () => {
        for (const [type, status] of Object.entries(LIST_VIEW_REGISTRY)) {
            if (status.onViewPath) continue;
            expect(status.reason.trim().length, `${type} needs a reason`).toBeGreaterThan(10);
        }
    });
});
