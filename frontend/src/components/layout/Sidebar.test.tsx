// @vitest-environment jsdom

import React from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Sidebar from './Sidebar';

const bridge = vi.hoisted(() => ({
    server: false,
    open: vi.fn(),
}));

vi.mock('wailsjs/runtime/runtime', () => ({ Environment: () => Promise.resolve({ platform: 'linux' }) }));
vi.mock('wailsjs/go/main/App', () => ({
    GetVersionInfo: () => Promise.resolve({}),
    IsDebugClusterEnabled: () => Promise.resolve(false),
}));
vi.mock('~/lib/wailsjs-adapter/runtime/runtime', () => ({
    isInServerMode: () => bridge.server,
    Environment: () => Promise.resolve({ platform: 'linux' }),
}));
vi.mock('~/lib/wailsjs-adapter/go/main/App', () => ({
    OpenAcceleratorBrowser: bridge.open,
    GetVersionInfo: () => Promise.resolve({}),
    IsDebugClusterEnabled: () => Promise.resolve(false),
}));
vi.mock('~/context', () => ({
    useConfig: () => ({ openConfigEditor: vi.fn(), config: null, setConfig: vi.fn() }),
    useK8s: () => ({ crds: [], crdsLoading: false, ensureCRDsLoaded: vi.fn() }),
    useAIChat: () => ({ isOpen: false, togglePanel: vi.fn(), providerAvailable: false }),
}));
vi.mock('~/hooks/usePerformancePanel', () => ({ usePerformancePanel: () => ({ openPerformancePanel: vi.fn() }) }));
vi.mock('~/hooks/useDebugLogs', () => ({ useDebugLogs: () => ({ toggleDebug: vi.fn() }) }));
vi.mock('../shared/SearchSelect', () => ({ default: () => <div data-testid="context-select" /> }));
vi.mock('./ContextManager', () => ({ default: () => null }));
vi.mock('./DebugClusterPanel', () => ({ default: () => null }));
vi.mock('~/utils/Logger', () => ({ default: { error: vi.fn() } }));

const props = {
    activeView: 'pods',
    onViewChange: vi.fn(),
    contexts: ['ctx'],
    currentContext: 'ctx',
    onContextChange: vi.fn(),
};

describe('Sidebar Accelerator Browser action', () => {
    beforeEach(() => {
        bridge.server = false;
        bridge.open.mockReset();
        Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { value: vi.fn(), configurable: true });
    });

    it('opens Accelerator Browser once with generic states', async () => {
        let resolve!: (value: string) => void;
        bridge.open.mockReturnValue(new Promise<string>((done) => { resolve = done; }));
        const view = render(<Sidebar {...props} />);
        const button = screen.getByRole('button', { name: 'Open Accelerator Browser' });
		await act(async () => {
			fireEvent.click(button);
			fireEvent.click(button);
		});
        expect(bridge.open).toHaveBeenCalledTimes(1);
        expect(bridge.open).toHaveBeenCalledWith();
		expect(screen.getByRole('status').textContent).toBe('Opening…');
        await act(async () => resolve('opened'));
		expect(screen.getByRole('status').textContent).toBe('Opened');
        view.rerender(<Sidebar {...props} currentContext="next" />);
        expect(screen.queryByRole('status')).toBeNull();
    });

    it('shows only generic failure and stays absent outside desktop/current context', async () => {
        bridge.open.mockResolvedValue('unavailable');
        const view = render(<Sidebar {...props} />);
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Open Accelerator Browser' })));
		expect(screen.getByRole('status').textContent).toBe('Unable to open Accelerator Browser');
        bridge.server = true;
        view.rerender(<Sidebar {...props} />);
        expect(screen.queryByRole('button', { name: 'Open Accelerator Browser' })).toBeNull();
        bridge.server = false;
        view.rerender(<Sidebar {...props} currentContext="" />);
        expect(screen.queryByRole('button', { name: 'Open Accelerator Browser' })).toBeNull();
    });
});
