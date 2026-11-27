import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { LogLine } from './LogLine';

const ESC = '\x1b';
const render = (entry: any, props: Record<string, any> = {}) =>
    renderToStaticMarkup(
        <LogLine entry={entry} showTimestamps={false} searchTerm="" searchRegex={null} wrapLines={true} {...props} />,
    );

import { converter } from './logUtils';

describe('LogLine per-entry caching', () => {
    it('converts a given entry once however often it re-renders, per prefix mode', () => {
        const spy = vi.spyOn(converter, 'toHtml');
        const entry = { content: `${ESC}[31mred${ESC}[0m [web-1/main] x` };
        render(entry);
        render(entry);
        render(entry, { wrapLines: false, showTimestamps: true });
        expect(spy).toHaveBeenCalledTimes(1);
        render(entry, { parsePrefixes: true });
        render(entry, { parsePrefixes: true });
        expect(spy).toHaveBeenCalledTimes(2);
        spy.mockRestore();
    });

    it('produces identical HTML on repeat renders of the same entry', () => {
        const entry = { content: `${ESC}[33mwarn without reset` };
        expect(render(entry)).toBe(render(entry));
    });

    it('ignores search prop changes for non-matching lines but not for matching ones', () => {
        const compare = (LogLine as any).compare;
        const entry = { content: 'x' };
        const base = { entry, showTimestamps: false, wrapLines: true, searchTerm: 'a', searchRegex: /a/gi, lineId: 1, isMatch: false };
        expect(compare(base, { ...base, searchTerm: 'b', searchRegex: /b/gi })).toBe(true);
        expect(compare({ ...base, isMatch: true }, { ...base, isMatch: true, searchTerm: 'b', searchRegex: /b/gi })).toBe(false);
        expect(compare(base, { ...base, lineId: 2 })).toBe(false);
        expect(compare(base, { ...base, entry: { content: 'x' } })).toBe(false);
    });

    it('prefers explicit lineId and isMatch props over entry fields', () => {
        const html = render({ content: 'x', originalIndex: 1, isMatch: false }, { lineId: 42, isMatch: true });
        expect(html).toContain('data-log-idx="42"');
        expect(html).toContain('bg-yellow-500/10');
    });
});
