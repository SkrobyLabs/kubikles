import { describe, expect, it } from 'vitest';
import { readFileSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';
import { DENIED_METHODS, EXTRA_BULK_METHODS, FORBIDDEN_PREFIXES, HISTORY_SUFFIXES, isBulkMethod, LIST_PREFIX, YAML_SUFFIXES } from './bulkMethods';

const here = dirname(fileURLToPath(import.meta.url));
const goSource = readFileSync(join(here, '../../../../pkg/bulk/allowlist.go'), 'utf-8');
const appJs = readFileSync(join(here, '../../../wailsjs/go/main/App.js'), 'utf-8');

function goStringList(name) {
    const m = new RegExp(`var ${name} = \\[\\]string\\{([\\s\\S]*?)\\}`).exec(goSource);
    if (!m) throw new Error(`cannot find ${name} in allowlist.go`);
    return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
}

const appMethods = [...appJs.matchAll(/^export function (\w+)\s*\(/gm)].map((m) => m[1]);

describe('bulk method allowlist parity with pkg/bulk/allowlist.go', () => {
    it('declares the same rule data', () => {
        const prefix = /const listPrefix = "(\w+)"/.exec(goSource)?.[1];
        expect(prefix).toBe(LIST_PREFIX);
        expect(goStringList('yamlSuffixes')).toEqual(YAML_SUFFIXES);
        expect(goStringList('historySuffixes')).toEqual(HISTORY_SUFFIXES);
        expect(goStringList('extraAllowed')).toEqual(EXTRA_BULK_METHODS);
        expect(goStringList('forbiddenPrefixes')).toEqual(FORBIDDEN_PREFIXES);
        expect(goStringList('denied')).toEqual(DENIED_METHODS);
    });

    // Independent restatement of the Go rules, driven by the parsed Go data.
    const goAllowed = (name) => {
        if (goStringList('denied').includes(name)) return false;
        if (goStringList('forbiddenPrefixes').some((p) => name.startsWith(p))) return false;
        if (name.startsWith('List')) return true;
        if (name.startsWith('Get') && [...goStringList('yamlSuffixes'), ...goStringList('historySuffixes')].some((s) => name.endsWith(s))) return true;
        return goStringList('extraAllowed').includes(name);
    };

    it('agrees on every binding name', () => {
        expect(appMethods.length).toBeGreaterThan(100);
        const mismatches = appMethods.filter((n) => isBulkMethod(n) !== goAllowed(n));
        expect(mismatches).toEqual([]);
    });

    it('every extra exists as a binding and no mutating binding is bulk routable', () => {
        for (const n of EXTRA_BULK_METHODS) expect(appMethods).toContain(n);
        const bad = appMethods.filter((n) => isBulkMethod(n) && FORBIDDEN_PREFIXES.some((p) => n.startsWith(p)));
        expect(bad).toEqual([]);
    });

    it('does not route ListPodFiles', () => {
        expect(isBulkMethod('ListPodFiles')).toBe(false);
        expect(isBulkMethod('ListPods')).toBe(true);
    });

    it('routes every list function the resource hooks use', () => {
        const idx = readFileSync(join(here, '../../hooks/resources/index.ts'), 'utf-8');
        const used = new Set([...idx.matchAll(/\bList[A-Za-z]+\b/g)].map((m) => m[0]));
        expect(used.size).toBeGreaterThan(20);
        for (const n of used) expect(isBulkMethod(n), n).toBe(true);
        expect(isBulkMethod('ListCustomResources')).toBe(true);
    });
});
