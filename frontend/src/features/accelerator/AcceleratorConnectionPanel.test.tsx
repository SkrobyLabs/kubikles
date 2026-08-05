/** @vitest-environment jsdom */
import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import AcceleratorConnectionPanel from './AcceleratorConnectionPanel';

const retry = vi.fn();
let acceleratorStatus: any = { state: 'direct_only', enabled: false, namespace: '', available: false };
vi.mock('~/context', () => ({
  useConfig: () => ({ config: { accelerator: { enabledByDefault: false, defaultNamespace: '', connectionOverrides: [] } }, setConfig: vi.fn() }),
  useK8s: () => ({ currentContext: 'prod', setSelectedNamespaces: vi.fn() }),
  useUI: () => ({ navigateWithSearch: vi.fn() }),
  useAccelerator: () => ({ status: acceleratorStatus, enable: vi.fn(), retry, disable: vi.fn() }),
}));

describe('AcceleratorConnectionPanel', () => {
  beforeEach(() => {
    acceleratorStatus = { state: 'direct_only', enabled: false, namespace: '', available: false };
    retry.mockReset();
  });

  it('links inheritance guidance to global Accelerator settings', () => {
    const onOpenSettings = vi.fn();
    render(<AcceleratorConnectionPanel contextName="prod" contextNamespace="default" onOpenSettings={onOpenSettings} />);
    expect(screen.queryByText('Enabled by')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Open Settings > Accelerator' }));
    expect(onOpenSettings).toHaveBeenCalledOnce();
  });

  it('shows deployment failures and makes unavailable deployments retryable', async () => {
    acceleratorStatus = { state: 'unavailable', enabled: true, namespace: 'default', available: false, diagnostics: [{ timestamp: '2026-08-05T12:00:00Z', phase: 'release resolution', reason: 'invalid_local_build', attempt: 1 }] };
    render(<AcceleratorConnectionPanel contextName="prod" contextNamespace="default" onOpenSettings={vi.fn()} />);
    expect(screen.getByText('Deployment log')).toBeTruthy();
    expect(screen.getByText(/does not have a publishable Accelerator release version/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Redeploy' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(retry).toHaveBeenCalledOnce());
  });
});
