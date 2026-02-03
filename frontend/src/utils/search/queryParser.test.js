import { describe, it, expect } from 'vitest';
import { parseQuery } from './queryParser';

describe('parseQuery', () => {
    describe('edge cases', () => {
        it('returns empty groups for null/undefined', () => {
            expect(parseQuery(null)).toEqual({ groups: [] });
            expect(parseQuery(undefined)).toEqual({ groups: [] });
        });

        it('returns empty groups for empty string', () => {
            expect(parseQuery('')).toEqual({ groups: [] });
            expect(parseQuery('   ')).toEqual({ groups: [] });
        });

        it('returns empty groups for non-string input', () => {
            expect(parseQuery(123)).toEqual({ groups: [] });
            expect(parseQuery({})).toEqual({ groups: [] });
        });
    });

    describe('plain text queries', () => {
        it('parses single word as plain condition', () => {
            const result = parseQuery('nginx');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toEqual([
                { type: 'plain', value: 'nginx', isRegex: false }
            ]);
        });

        it('parses multiple words as separate plain conditions', () => {
            const result = parseQuery('nginx redis');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toEqual([
                { type: 'plain', value: 'nginx', isRegex: false },
                { type: 'plain', value: 'redis', isRegex: false }
            ]);
        });

        it('handles extra whitespace', () => {
            const result = parseQuery('  nginx   redis  ');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toHaveLength(2);
            expect(result.groups[0][0].value).toBe('nginx');
            expect(result.groups[0][1].value).toBe('redis');
        });
    });

    describe('field-specific queries', () => {
        it('parses unquoted field value', () => {
            const result = parseQuery('name:nginx');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toEqual([
                { type: 'field', field: 'name', value: 'nginx', isRegex: false }
            ]);
        });

        it('parses double-quoted field value', () => {
            const result = parseQuery('name:"my-pod"');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toEqual([
                { type: 'field', field: 'name', value: 'my-pod', isRegex: false }
            ]);
        });

        it('parses single-quoted field value', () => {
            const result = parseQuery("name:'my-pod'");
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toEqual([
                { type: 'field', field: 'name', value: 'my-pod', isRegex: false }
            ]);
        });

        it('handles empty quoted values', () => {
            const result = parseQuery('name:""');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toEqual([
                { type: 'field', field: 'name', value: '', isRegex: false }
            ]);
        });

        it('lowercases field names', () => {
            const result = parseQuery('Name:nginx NodeName:node1');
            expect(result.groups[0][0].field).toBe('name');
            expect(result.groups[0][1].field).toBe('nodename');
        });

        it('parses multiple field conditions', () => {
            const result = parseQuery('name:nginx status:Running');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toHaveLength(2);
            expect(result.groups[0][0]).toEqual({ type: 'field', field: 'name', value: 'nginx', isRegex: false });
            expect(result.groups[0][1]).toEqual({ type: 'field', field: 'status', value: 'Running', isRegex: false });
        });
    });

    describe('regex queries', () => {
        it('parses regex pattern', () => {
            const result = parseQuery('name:/^nginx/');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toHaveLength(1);
            expect(result.groups[0][0].type).toBe('field');
            expect(result.groups[0][0].field).toBe('name');
            expect(result.groups[0][0].isRegex).toBe(true);
            expect(result.groups[0][0].value).toBeInstanceOf(RegExp);
            expect(result.groups[0][0].value.source).toBe('^nginx');
        });

        it('parses regex with flags', () => {
            const result = parseQuery('name:/nginx/i');
            expect(result.groups[0][0].value.flags).toBe('i');
        });

        it('parses regex with multiple flags', () => {
            const result = parseQuery('name:/pattern/gi');
            expect(result.groups[0][0].value.flags).toContain('g');
            expect(result.groups[0][0].value.flags).toContain('i');
        });

        it('handles complex regex patterns', () => {
            const result = parseQuery('name:/^(api|web)-\\d+$/');
            expect(result.groups[0][0].value.test('api-123')).toBe(true);
            expect(result.groups[0][0].value.test('web-456')).toBe(true);
            expect(result.groups[0][0].value.test('db-789')).toBe(false);
        });

        it('treats invalid regex as literal string', () => {
            const result = parseQuery('name:/[invalid/');
            expect(result.groups[0][0].isRegex).toBe(false);
            expect(result.groups[0][0].value).toBe('/[invalid/');
        });
    });

    describe('mixed queries', () => {
        it('parses plain text with field conditions', () => {
            const result = parseQuery('api name:nginx status:Running');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toHaveLength(3);
            expect(result.groups[0][0]).toEqual({ type: 'plain', value: 'api', isRegex: false });
            expect(result.groups[0][1]).toEqual({ type: 'field', field: 'name', value: 'nginx', isRegex: false });
            expect(result.groups[0][2]).toEqual({ type: 'field', field: 'status', value: 'Running', isRegex: false });
        });

        it('parses field conditions with regex', () => {
            const result = parseQuery('name:"nginx" status:/Running|Pending/');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toHaveLength(2);
            expect(result.groups[0][0].isRegex).toBe(false);
            expect(result.groups[0][1].isRegex).toBe(true);
        });

        it('handles quoted values with spaces', () => {
            const result = parseQuery('label:"app = nginx"');
            expect(result.groups[0][0].value).toBe('app = nginx');
        });
    });

    describe('OR groups', () => {
        it('splits conditions by OR keyword', () => {
            const result = parseQuery('name:/^web-/ OR name:/^api-/');
            expect(result.groups).toHaveLength(2);
            expect(result.groups[0]).toHaveLength(1);
            expect(result.groups[1]).toHaveLength(1);
            expect(result.groups[0][0].field).toBe('name');
            expect(result.groups[1][0].field).toBe('name');
        });

        it('handles multiple OR groups', () => {
            const result = parseQuery('status:Running OR status:Pending OR status:Succeeded');
            expect(result.groups).toHaveLength(3);
        });
    });

    describe('real-world examples', () => {
        it('parses pod search query', () => {
            const result = parseQuery('name:nginx namespace:default status:/Running|Pending/');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toHaveLength(3);
            expect(result.groups[0][0]).toEqual({ type: 'field', field: 'name', value: 'nginx', isRegex: false });
            expect(result.groups[0][1]).toEqual({ type: 'field', field: 'namespace', value: 'default', isRegex: false });
            expect(result.groups[0][2].isRegex).toBe(true);
        });

        it('parses node search query', () => {
            const result = parseQuery('nodeName:/^worker-/ status:Ready');
            expect(result.groups).toHaveLength(1);
            expect(result.groups[0]).toHaveLength(2);
            expect(result.groups[0][0].value.test('worker-1')).toBe(true);
            expect(result.groups[0][0].value.test('master-1')).toBe(false);
        });
    });
});
