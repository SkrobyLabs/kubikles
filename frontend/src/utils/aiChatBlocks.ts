// Pure reducer helpers for composing an assistant chat turn out of ordered
// blocks: text segments interleaved with tool-call cards. Every function returns
// a NEW array and never mutates its inputs, so it is safe to use inside React
// state updaters and trivially unit-testable.

export type ToolCallStatus = 'running' | 'done' | 'error';

export interface TextBlock {
    type: 'text';
    content: string;
}

export interface ToolCallBlock {
    type: 'tool_call';
    toolId: string;
    name: string;
    input?: string;
    result?: string;
    status: ToolCallStatus;
}

export type MessageBlock = TextBlock | ToolCallBlock;

export interface ToolEventPayload {
    id: string;
    name: string;
    input?: string;
    result?: string;
    error?: boolean;
    done?: boolean;
}

// appendText appends a text chunk to the trailing text block if the last block
// is text, otherwise pushes a new text block (e.g. after a tool-call card).
export function appendText(blocks: MessageBlock[], chunk: string): MessageBlock[] {
    const last = blocks[blocks.length - 1];
    if (last && last.type === 'text') {
        return [
            ...blocks.slice(0, -1),
            { type: 'text', content: last.content + chunk },
        ];
    }
    return [...blocks, { type: 'text', content: chunk }];
}

// applyToolEvent folds a tool lifecycle event into the block list. A start event
// (done falsy) pushes a running tool_call block; a done event updates the matching
// block by toolId, or pushes a completed block if the start was missed.
export function applyToolEvent(blocks: MessageBlock[], ev: ToolEventPayload): MessageBlock[] {
    const finalStatus: ToolCallStatus = ev.error ? 'error' : 'done';

    if (!ev.done) {
        return [
            ...blocks,
            { type: 'tool_call', toolId: ev.id, name: ev.name, input: ev.input, status: 'running' },
        ];
    }

    let matched = false;
    const updated = blocks.map(block => {
        if (block.type === 'tool_call' && block.toolId === ev.id) {
            matched = true;
            return { ...block, result: ev.result, status: finalStatus };
        }
        return block;
    });
    if (matched) {
        return updated;
    }
    // No prior tool_use seen — surface the completed call anyway.
    return [
        ...blocks,
        { type: 'tool_call', toolId: ev.id, name: ev.name, result: ev.result, status: finalStatus },
    ];
}

// finalizeBlocks clears the running state on any lingering tool blocks so no
// spinner is left behind. On a normal completion pass 'done'; on an
// interrupted turn (cancel/error) pass 'error' so a mid-flight tool is not
// shown as if it had succeeded.
export function finalizeBlocks(blocks: MessageBlock[], interruptedStatus: ToolCallStatus = 'done'): MessageBlock[] {
    return blocks.map(block =>
        block.type === 'tool_call' && block.status === 'running'
            ? { ...block, status: interruptedStatus }
            : block
    );
}

// blocksToText concatenates the text blocks (for copy + persistence).
export function blocksToText(blocks: MessageBlock[]): string {
    return blocks
        .filter((b): b is TextBlock => b.type === 'text')
        .map(b => b.content)
        .join('');
}
