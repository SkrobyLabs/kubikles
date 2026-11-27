import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { LogLine } from './LogLine';

const ESC = '\x1b';
const render = (entry: any, props: Record<string, any> = {}) =>
    renderToStaticMarkup(
        <LogLine entry={entry} showTimestamps={false} searchTerm="" searchRegex={null} wrapLines={true} {...props} />,
    );
const contentOf = (html: string) => /<span data-log-content="">([\s\S]*)<\/span><\/div>$/.exec(html)![1];

describe('LogLine ANSI to HTML', () => {
    it('renders plain text unchanged and escapes markup', () => {
        expect(contentOf(render({ content: 'hello world' }))).toBe('hello world');
        expect(contentOf(render({ content: '<b>x</b> & "y"' }))).toBe('&lt;b&gt;x&lt;/b&gt; &amp; &quot;y&quot;');
    });

    it('converts foreground colours to styled spans', () => {
        expect(contentOf(render({ content: `${ESC}[31mred${ESC}[0m plain` }))).toBe('<span style="color:#A00">red</span> plain');
        expect(contentOf(render({ content: `${ESC}[32mgreen${ESC}[0m` }))).toBe('<span style="color:#0A0">green</span>');
    });

    it('converts bold, background and 256-colour codes', () => {
        expect(contentOf(render({ content: `${ESC}[1mbold${ESC}[0m` }))).toBe('<b>bold</b>');
        expect(contentOf(render({ content: `${ESC}[41mbg${ESC}[0m` }))).toBe('<span style="background-color:#A00">bg</span>');
        expect(contentOf(render({ content: `${ESC}[38;5;196mhot${ESC}[0m` }))).toBe('<span style="color:#ff0000">hot</span>');
    });

    it('normalises non-standard zero-padded 256-colour codes before converting', () => {
        const padded = contentOf(render({ content: `${ESC}[38;5;0196mhot${ESC}[0m` }));
        const standard = contentOf(render({ content: `${ESC}[38;5;196mhot${ESC}[0m` }));
        expect(padded).toBe(standard);
    });

    it('closes spans that were left open at end of line', () => {
        expect(contentOf(render({ content: `${ESC}[33mwarn without reset` }))).toBe('<span style="color:#A50">warn without reset</span>');
    });

    it('turns embedded newlines into line breaks', () => {
        expect(contentOf(render({ content: 'a\nb' }))).toBe('a<br/>b');
    });

    it('handles empty content', () => {
        expect(contentOf(render({ content: '' }))).toBe('');
    });
});

describe('LogLine layout attributes', () => {
    it('exposes the original index and wrap mode on the row', () => {
        const wrapped = render({ content: 'x', originalIndex: 7 });
        expect(wrapped).toContain('data-log-idx="7"');
        expect(wrapped).toContain('whitespace-pre-wrap break-all');
        const unwrapped = render({ content: 'x', originalIndex: 7 }, { wrapLines: false });
        expect(unwrapped).toContain('whitespace-pre');
        expect(unwrapped).not.toContain('break-all');
    });

    it('shows the timestamp only when requested and available', () => {
        const entry = { content: 'x', timestamp: '2024-05-01T10:00:00.000000000Z' };
        expect(render(entry, { showTimestamps: true })).toContain('2024-05-01T10:00:00.000000000Z');
        expect(render(entry, { showTimestamps: false })).not.toContain('2024-05-01');
        expect(render({ content: 'x', timestamp: '' }, { showTimestamps: true })).not.toContain('text-gray-500 select-none mr-2');
    });

    it('renders a skip indicator with correct pluralisation', () => {
        expect(render({ isSkipIndicator: true, skippedCount: 1 })).toContain('... skipped 1 line ...');
        expect(render({ isSkipIndicator: true, skippedCount: 12 })).toContain('... skipped 12 lines ...');
    });
});

describe('LogLine search highlighting', () => {
    const regex = (source: string) => new RegExp(source, 'gi');

    it('highlights matches only for entries flagged as matches', () => {
        const entry = { content: 'an error occurred', isMatch: true };
        const html = contentOf(render(entry, { searchTerm: 'error', searchRegex: regex('error') }));
        expect(html).toContain('<mark');
        expect(html.replace(/<[^>]+>/g, '')).toBe('an error occurred');
        expect(contentOf(render({ content: 'an error occurred', isMatch: false }, { searchTerm: 'error', searchRegex: regex('error') })))
            .toBe('an error occurred');
    });

    it('highlights across an ANSI colour boundary using the stripped text', () => {
        const entry = { content: `${ESC}[31mERR${ESC}[0m]text`, isMatch: true };
        const html = contentOf(render(entry, { searchTerm: 'ERR]text', searchRegex: regex('ERR\\]text') }));
        expect(html).toContain('<mark');
        expect(html.replace(/<[^>]+>/g, '')).toBe('ERR]text');
        expect(html).toContain('color:#A00');
    });

    it('leaves content alone when there is a term but no regex', () => {
        expect(contentOf(render({ content: 'abc', isMatch: true }, { searchTerm: 'abc', searchRegex: null }))).toBe('abc');
    });
});

describe('LogLine multi-source prefixes', () => {
    const entry = { content: '[web-1/main] started ok' };

    it('does not parse prefixes unless requested', () => {
        const html = render(entry);
        expect(contentOf(html)).toBe('[web-1/main] started ok');
        expect(html).not.toContain('select-none mr-1');
    });

    it('splits a pod/container prefix into coloured parts and strips it from content', () => {
        const html = render(entry, { parsePrefixes: true });
        expect(contentOf(html)).toBe('started ok');
        expect(html).toMatch(/\[<span style="color:#[0-9a-f]{6}">web-1<\/span><span class="text-gray-500">\/<\/span><span style="color:#[0-9a-f]{6}">main<\/span>\]/);
    });

    it('renders a single-name prefix with one colour', () => {
        const html = render({ content: '[sidecar] hi' }, { parsePrefixes: true });
        expect(contentOf(html)).toBe('hi');
        expect(html).toMatch(/style="color:#[0-9a-f]{6}">\[sidecar\]</);
    });

    it('assigns a stable colour per name', () => {
        const colour = (html: string) => /color:(#[0-9a-f]{6})">web-1/.exec(html)![1];
        expect(colour(render(entry, { parsePrefixes: true }))).toBe(colour(render({ content: '[web-1/other] x' }, { parsePrefixes: true })));
    });

    it('does not treat application log levels or capitalised tags as prefixes', () => {
        for (const content of ['[INFO] boot', '[Microsoft.Hosting.Lifetime] up']) {
            const html = render({ content }, { parsePrefixes: true });
            expect(contentOf(html)).toBe(content);
            expect(html).not.toContain('select-none mr-1');
        }
    });

    it('QUIRK: an all-lowercase/digit tag such as a bracketed date is parsed as a prefix', () => {
        const html = render({ content: '[2024-05-01] hello' }, { parsePrefixes: true });
        expect(contentOf(html)).toBe('hello');
        expect(html).toContain('[2024-05-01]');
    });
});
