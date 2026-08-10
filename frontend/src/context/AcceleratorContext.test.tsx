/** @vitest-environment jsdom */
import React from 'react';
import { render, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AcceleratorProvider } from './AcceleratorContext';

const { bridge, logger } = vi.hoisted(() => ({
  bridge: {
    DisableAccelerator: vi.fn(),
    EnableAccelerator: vi.fn(),
    GetAcceleratorStatus: vi.fn(),
    RetryAccelerator: vi.fn(),
  },
  logger: { error: vi.fn(), info: vi.fn(), warn: vi.fn(), debug: vi.fn() },
}));

vi.mock('~/lib/wailsjs-adapter/go/main/App', () => bridge);
vi.mock('./ConfigContext', () => ({ useConfig: () => ({ config: { accelerator: { enabledByDefault: false, defaultNamespace: '', connectionOverrides: [] } } }) }));
vi.mock('./K8sContext', () => ({ useK8s: () => ({ currentContext: 'selected', currentNamespace: 'default' }) }));
vi.mock('~/utils/Logger', () => ({ default: logger }));

describe('AcceleratorProvider diagnostics', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'go', { configurable: true, value: {} });
    vi.clearAllMocks();
  });

  afterEach(() => {
    delete (window as any).go;
  });

  it('does not mirror coordinator diagnostics into the frontend Debug log', async () => {
    bridge.GetAcceleratorStatus.mockResolvedValue({ state: 'unavailable', enabled: true, namespace: 'default', available: false, diagnostics: [{ timestamp: '2026-08-10T12:00:00Z', phase: 'provisioning', reason: 'context_unavailable', attempt: 1 }] });
    render(<AcceleratorProvider><div /></AcceleratorProvider>);

    await waitFor(() => expect(bridge.GetAcceleratorStatus).toHaveBeenCalledWith('selected'));
    expect(logger.error).not.toHaveBeenCalledWith('Accelerator deployment failed', expect.anything(), 'helm');
  });

  it('continues to log status request failures', async () => {
    bridge.GetAcceleratorStatus.mockRejectedValue(new Error('offline'));
    render(<AcceleratorProvider><div /></AcceleratorProvider>);

    await waitFor(() => expect(logger.error).toHaveBeenCalledWith('Failed to read Accelerator deployment status', expect.any(Error), 'helm'));
  });
});
