import { describe, it, expect } from 'vitest';
import {
    appendText,
    applyToolEvent,
    finalizeBlocks,
    blocksToText,
    MessageBlock,
} from './aiChatBlocks';

describe('appendText', () => {
    it('creates a text block from empty', () => {
        expect(appendText([], 'hello')).toEqual([{ type: 'text', content: 'hello' }]);
    });

    it('merges into a trailing text block', () => {
        const blocks: MessageBlock[] = [{ type: 'text', content: 'hel' }];
        expect(appendText(blocks, 'lo')).toEqual([{ type: 'text', content: 'hello' }]);
    });

    it('pushes a new text block after a tool block', () => {
        const blocks: MessageBlock[] = [
            { type: 'text', content: 'before' },
            { type: 'tool_call', toolId: 't1', name: 'get_pod_logs', status: 'done' },
        ];
        const result = appendText(blocks, 'after');
        expect(result).toHaveLength(3);
        expect(result[2]).toEqual({ type: 'text', content: 'after' });
    });

    it('does not mutate the input array', () => {
        const blocks: MessageBlock[] = [{ type: 'text', content: 'a' }];
        appendText(blocks, 'b');
        expect(blocks).toEqual([{ type: 'text', content: 'a' }]);
    });
});

describe('applyToolEvent', () => {
    it('pushes a running block on start', () => {
        const result = applyToolEvent([], { id: 't1', name: 'get_pod_logs', input: '{"pod":"x"}' });
        expect(result).toEqual([
            { type: 'tool_call', toolId: 't1', name: 'get_pod_logs', input: '{"pod":"x"}', status: 'running' },
        ]);
    });

    it('correlates a result to the running block by id', () => {
        const started = applyToolEvent([], { id: 't1', name: 'get_pod_logs', input: '{}' });
        const done = applyToolEvent(started, { id: 't1', name: 'get_pod_logs', result: 'logs...', done: true });
        expect(done).toHaveLength(1);
        expect(done[0]).toEqual({
            type: 'tool_call',
            toolId: 't1',
            name: 'get_pod_logs',
            input: '{}',
            result: 'logs...',
            status: 'done',
        });
    });

    it('marks the block as error when the result is an error', () => {
        const started = applyToolEvent([], { id: 't1', name: 'bad', input: '{}' });
        const done = applyToolEvent(started, { id: 't1', name: 'bad', result: 'boom', error: true, done: true });
        expect((done[0] as any).status).toBe('error');
        expect((done[0] as any).result).toBe('boom');
    });

    it('correlates a result to the matching id when several tools run concurrently', () => {
        let blocks = applyToolEvent([], { id: 't1', name: 'a', input: '{}' });
        blocks = applyToolEvent(blocks, { id: 't2', name: 'b', input: '{}' });
        // Resolve only t1 — t2 must stay running.
        blocks = applyToolEvent(blocks, { id: 't1', name: 'a', result: 'done-a', done: true });
        expect(blocks).toHaveLength(2);
        expect(blocks[0]).toMatchObject({ toolId: 't1', status: 'done', result: 'done-a' });
        expect(blocks[1]).toMatchObject({ toolId: 't2', status: 'running' });
    });

    it('pushes a completed block when the result arrives without a prior start', () => {
        const result = applyToolEvent([], { id: 't9', name: 'orphan', result: 'r', done: true });
        expect(result).toEqual([
            { type: 'tool_call', toolId: 't9', name: 'orphan', result: 'r', status: 'done' },
        ]);
    });

    it('does not mutate the input array', () => {
        const blocks: MessageBlock[] = [{ type: 'tool_call', toolId: 't1', name: 'x', status: 'running' }];
        applyToolEvent(blocks, { id: 't1', name: 'x', result: 'r', done: true });
        expect((blocks[0] as any).status).toBe('running');
    });
});

describe('finalizeBlocks', () => {
    it('clears running status on lingering tool blocks', () => {
        const blocks: MessageBlock[] = [
            { type: 'text', content: 'hi' },
            { type: 'tool_call', toolId: 't1', name: 'x', status: 'running' },
            { type: 'tool_call', toolId: 't2', name: 'y', status: 'error' },
        ];
        const result = finalizeBlocks(blocks);
        expect((result[1] as any).status).toBe('done');
        expect((result[2] as any).status).toBe('error');
        expect(result[0]).toEqual({ type: 'text', content: 'hi' });
    });

    it('marks lingering running tools with the interrupted status on cancel/error', () => {
        const blocks: MessageBlock[] = [
            { type: 'tool_call', toolId: 't1', name: 'x', status: 'running' },
        ];
        const result = finalizeBlocks(blocks, 'error');
        expect((result[0] as any).status).toBe('error');
    });
});

describe('blocksToText', () => {
    it('concatenates only text blocks', () => {
        const blocks: MessageBlock[] = [
            { type: 'text', content: 'Hello ' },
            { type: 'tool_call', toolId: 't1', name: 'x', status: 'done' },
            { type: 'text', content: 'world' },
        ];
        expect(blocksToText(blocks)).toBe('Hello world');
    });

    it('returns empty string for no text blocks', () => {
        expect(blocksToText([{ type: 'tool_call', toolId: 't1', name: 'x', status: 'done' }])).toBe('');
    });
});
