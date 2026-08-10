/** @vitest-environment jsdom */
import React from 'react';
import { act, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { bridge, runtime } = vi.hoisted(() => ({
    bridge: { SetDebugEnabled: vi.fn(() => Promise.resolve()) },
    runtime: {
        handler: undefined as ((payload: unknown, message?: unknown, details?: unknown) => void) | undefined,
        EventsOn: vi.fn((_name: string, handler: (payload: unknown, message?: unknown, details?: unknown) => void) => {
            runtime.handler = handler;
            return vi.fn();
        }),
    },
}));

vi.mock('wailsjs/go/main/App', () => bridge);
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: runtime.EventsOn }));

import { DebugProvider, useDebug } from './DebugContext';

function DebugProbe(): React.ReactElement {
    const { logs } = useDebug();
    return <pre data-testid="logs">{JSON.stringify(logs)}</pre>;
}

describe('DebugProvider backend events', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        runtime.handler = undefined;
        localStorage.clear();
        localStorage.setItem('kubikles-debug-enabled', 'true');
    });

    it('accepts the structured backend payload used by desktop and server transports', async () => {
        render(<DebugProvider><DebugProbe /></DebugProvider>);

        await waitFor(() => expect(bridge.SetDebugEnabled).toHaveBeenCalledWith(true));
        expect(runtime.handler).toBeTypeOf('function');
        act(() => runtime.handler?.({
            category: 'helm',
            message: 'Accelerator chart pull failed',
            details: { stage: 'registry_pull', error: 'connection refused' },
        }));

        const logs = JSON.parse(screen.getByTestId('logs').textContent || '[]');
        expect(logs).toHaveLength(1);
        expect(logs[0]).toMatchObject({
            source: 'be',
            category: 'helm',
            message: 'Accelerator chart pull failed',
            details: { stage: 'registry_pull', error: 'connection refused' },
        });
    });
});
