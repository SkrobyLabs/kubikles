import { describe, it, expect } from 'vitest';
import {
    normalizeAnsiCodes,
    stripAnsiCodes,
    isValidDateTime,
    toRFC3339,
    parseLogLines,
    highlightMatchesInHtml,
    logsToVisibleString,
    logsToDebugString
} from './logUtils';

describe('normalizeAnsiCodes', () => {
    it('normalizes 4-digit foreground color codes', () => {
        const input = '\x1b[38;5;0008m';
        const expected = '\x1b[38;5;8m';
        expect(normalizeAnsiCodes(input)).toBe(expected);
    });

    it('normalizes 4-digit background color codes', () => {
        const input = '\x1b[48;5;0128m';
        const expected = '\x1b[48;5;128m';
        expect(normalizeAnsiCodes(input)).toBe(expected);
    });

    it('leaves standard codes unchanged', () => {
        const input = '\x1b[38;5;8mtest\x1b[0m';
        expect(normalizeAnsiCodes(input)).toBe(input);
    });

    it('handles multiple non-standard codes in one string', () => {
        const input = '\x1b[38;5;0001mred\x1b[38;5;0002mgreen';
        const expected = '\x1b[38;5;1mred\x1b[38;5;2mgreen';
        expect(normalizeAnsiCodes(input)).toBe(expected);
    });

    it('returns plain text unchanged', () => {
        const input = 'plain text without codes';
        expect(normalizeAnsiCodes(input)).toBe(input);
    });
});

describe('stripAnsiCodes', () => {
    it('strips simple color codes', () => {
        const input = '\x1b[31mred text\x1b[0m';
        expect(stripAnsiCodes(input)).toBe('red text');
    });

    it('strips 256-color codes', () => {
        const input = '\x1b[38;5;196mtext\x1b[0m';
        expect(stripAnsiCodes(input)).toBe('text');
    });

    it('strips multiple codes', () => {
        const input = '\x1b[1m\x1b[31mbold red\x1b[0m normal';
        expect(stripAnsiCodes(input)).toBe('bold red normal');
    });

    it('returns plain text unchanged', () => {
        const input = 'no escape codes here';
        expect(stripAnsiCodes(input)).toBe(input);
    });

    it('handles empty string', () => {
        expect(stripAnsiCodes('')).toBe('');
    });
});

describe('isValidDateTime', () => {
    it('accepts RFC3339 format with Z suffix', () => {
        expect(isValidDateTime('2024-11-26T14:30:00Z')).toBe(true);
    });

    it('accepts RFC3339 format without Z suffix', () => {
        expect(isValidDateTime('2024-11-26T14:30:00')).toBe(true);
    });

    it('accepts space-separated format', () => {
        expect(isValidDateTime('2024-11-26 14:30:00')).toBe(true);
    });

    it('accepts short format with T', () => {
        expect(isValidDateTime('2024-11-26T14:30')).toBe(true);
    });

    it('accepts short format with space', () => {
        expect(isValidDateTime('2024-11-26 14:30')).toBe(true);
    });

    it('rejects invalid date', () => {
        expect(isValidDateTime('2024-13-45T99:99:99Z')).toBe(false);
    });

    it('rejects random strings', () => {
        expect(isValidDateTime('not a date')).toBe(false);
    });

    it('rejects null/undefined', () => {
        expect(isValidDateTime(null)).toBe(false);
        expect(isValidDateTime(undefined)).toBe(false);
        expect(isValidDateTime('')).toBe(false);
    });
});

describe('toRFC3339', () => {
    it('adds Z suffix to complete datetime', () => {
        expect(toRFC3339('2024-11-26T14:30:00')).toBe('2024-11-26T14:30:00Z');
    });

    it('adds :00Z to short datetime', () => {
        expect(toRFC3339('2024-11-26T14:30')).toBe('2024-11-26T14:30:00Z');
    });

    it('converts space to T', () => {
        expect(toRFC3339('2024-11-26 14:30:00')).toBe('2024-11-26T14:30:00Z');
    });

    it('leaves already valid RFC3339 unchanged', () => {
        expect(toRFC3339('2024-11-26T14:30:00Z')).toBe('2024-11-26T14:30:00Z');
    });

    it('handles empty string', () => {
        expect(toRFC3339('')).toBe('');
    });

    it('handles null/undefined', () => {
        expect(toRFC3339(null)).toBe('');
        expect(toRFC3339(undefined)).toBe('');
    });
});

describe('parseLogLines', () => {
    it('parses K8s timestamped logs', () => {
        const raw = '2024-11-26T14:30:00.123456789Z Hello world';
        const result = parseLogLines(raw, 'initial');
        expect(result).toEqual([{
            timestamp: '2024-11-26T14:30:00.123456789Z',
            content: 'Hello world',
            source: 'initial'
        }]);
    });

    it('parses multiple log lines', () => {
        const raw = `2024-11-26T14:30:00.123456789Z Line 1
2024-11-26T14:30:01.123456789Z Line 2`;
        const result = parseLogLines(raw, 'stream');
        expect(result).toHaveLength(2);
        expect(result[0].content).toBe('Line 1');
        expect(result[1].content).toBe('Line 2');
    });

    it('handles lines without timestamps', () => {
        const raw = 'No timestamp here';
        const result = parseLogLines(raw, 'after');
        expect(result).toEqual([{
            timestamp: '',
            content: 'No timestamp here',
            source: 'after'
        }]);
    });

    it('filters empty lines', () => {
        const raw = `2024-11-26T14:30:00.123456789Z Line 1

2024-11-26T14:30:01.123456789Z Line 2
   `;
        const result = parseLogLines(raw, 'initial');
        expect(result).toHaveLength(2);
    });

    it('handles empty input', () => {
        expect(parseLogLines('', 'initial')).toEqual([]);
        expect(parseLogLines(null, 'initial')).toEqual([]);
        expect(parseLogLines(undefined, 'initial')).toEqual([]);
    });

    it('preserves ANSI codes in content', () => {
        const raw = '2024-11-26T14:30:00.123456789Z \x1b[31mRed text\x1b[0m';
        const result = parseLogLines(raw, 'initial');
        expect(result[0].content).toBe('\x1b[31mRed text\x1b[0m');
    });
});

describe('highlightMatchesInHtml', () => {
    it('highlights simple text match', () => {
        const html = 'hello world';
        const plain = 'hello world';
        const regex = /world/g;
        const result = highlightMatchesInHtml(html, plain, regex);
        expect(result).toBe('hello <mark class="bg-yellow-500/50 text-inherit">world</mark>');
    });

    it('highlights multiple matches', () => {
        const html = 'foo bar foo';
        const plain = 'foo bar foo';
        const regex = /foo/g;
        const result = highlightMatchesInHtml(html, plain, regex);
        expect(result).toBe('<mark class="bg-yellow-500/50 text-inherit">foo</mark> bar <mark class="bg-yellow-500/50 text-inherit">foo</mark>');
    });

    it('handles HTML entities', () => {
        const html = '&lt;div&gt;';
        const plain = '<div>';
        const regex = /div/g;
        const result = highlightMatchesInHtml(html, plain, regex);
        expect(result).toBe('&lt;<mark class="bg-yellow-500/50 text-inherit">div</mark>&gt;');
    });

    it('preserves span tags from ANSI conversion', () => {
        const html = '<span style="color:#F00">red</span> text';
        const plain = 'red text';
        const regex = /red/g;
        const result = highlightMatchesInHtml(html, plain, regex);
        expect(result).toContain('<mark');
        expect(result).toContain('</mark>');
        expect(result).toContain('<span style="color:#F00">');
    });

    it('returns original HTML if no regex provided', () => {
        const html = 'test text';
        expect(highlightMatchesInHtml(html, 'test text', null)).toBe(html);
    });

    it('returns original HTML if no matches', () => {
        const html = 'hello world';
        const result = highlightMatchesInHtml(html, 'hello world', /xyz/g);
        expect(result).toBe('hello world');
    });
});

describe('logsToVisibleString', () => {
    const logs = [
        { timestamp: '2024-11-26T14:30:00.123Z', content: 'Line 1', source: 'initial' },
        { timestamp: '2024-11-26T14:30:01.123Z', content: 'Line 2', source: 'stream' }
    ];

    it('includes timestamps when showTimestamps is true', () => {
        const result = logsToVisibleString(logs, true);
        expect(result).toBe('2024-11-26T14:30:00.123Z Line 1\n2024-11-26T14:30:01.123Z Line 2');
    });

    it('excludes timestamps when showTimestamps is false', () => {
        const result = logsToVisibleString(logs, false);
        expect(result).toBe('Line 1\nLine 2');
    });

    it('handles logs without timestamps', () => {
        const noTimestampLogs = [
            { timestamp: '', content: 'No timestamp', source: 'initial' }
        ];
        const result = logsToVisibleString(noTimestampLogs, true);
        expect(result).toBe('No timestamp');
    });

    it('handles empty array', () => {
        expect(logsToVisibleString([], true)).toBe('');
    });
});

describe('logsToDebugString', () => {
    it('includes source markers', () => {
        const logs = [
            { timestamp: '2024-11-26T14:30:00.123Z', content: 'Line 1', source: 'initial' },
            { timestamp: '', content: 'Line 2', source: 'stream' }
        ];
        const result = logsToDebugString(logs);
        expect(result).toBe('2024-11-26T14:30:00.123Z [INITIAL] Line 1\n[STREAM] Line 2');
    });

    it('handles empty array', () => {
        expect(logsToDebugString([])).toBe('');
    });
});
