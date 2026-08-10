/** @vitest-environment jsdom */
import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import AcceleratorConnectionPanel from './AcceleratorConnectionPanel';

const retry = vi.fn();
const disable = vi.fn();
const setConfig = vi.fn();
const writeText = vi.fn();
let acceleratorStatus: any = { state: 'direct_only', enabled: false, namespace: '', available: false };
let acceleratorConfig: any = { enabledByDefault: false, defaultNamespace: '', customImage: '', customChart: '', connectionOverrides: [] };
vi.mock('~/context', () => ({
  useConfig: () => ({ config: { accelerator: acceleratorConfig }, setConfig }),
  useK8s: () => ({ currentContext: 'prod', setSelectedNamespaces: vi.fn() }),
  useUI: () => ({ navigateWithSearch: vi.fn() }),
  useAccelerator: () => ({ status: acceleratorStatus, enable: vi.fn(), retry, disable }),
}));

describe('AcceleratorConnectionPanel', () => {
  beforeEach(() => {
    acceleratorStatus = { state: 'direct_only', enabled: false, namespace: '', available: false };
    acceleratorConfig = { enabledByDefault: false, defaultNamespace: '', customImage: '', customChart: '', connectionOverrides: [] };
    retry.mockReset();
    disable.mockReset();
    setConfig.mockReset();
    writeText.mockReset();
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  });

  it('copies the visible deployment failures from the error panel', async () => {
    acceleratorStatus = { state: 'unavailable', enabled: true, namespace: 'default', available: false, diagnostics: [{ timestamp: '2026-08-05T12:00:00Z', phase: 'reconnection', reason: 'session_not_resume_eligible', attempt: 1 }] };
    render(<AcceleratorConnectionPanel contextName="prod" contextNamespace="default" onOpenSettings={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'Copy deployment log' }));
    await waitFor(() => expect(writeText).toHaveBeenCalledOnce());
    expect(writeText.mock.calls[0][0]).toContain('reconnection: session not resume eligible (attempt 1)');
    expect(screen.getByText('Copied')).toBeTruthy();
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

  it('disables a failed deployment without asking to remove a nonexistent workload', async () => {
    acceleratorStatus = { state: 'unavailable', enabled: true, namespace: 'default', available: false, diagnostics: [] };
    const confirm = vi.spyOn(window, 'confirm');
    render(<AcceleratorConnectionPanel contextName="prod" contextNamespace="default" onOpenSettings={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'Disable & Remove' }));
    await waitFor(() => expect(disable).toHaveBeenCalledOnce());
    expect(confirm).not.toHaveBeenCalled();
    expect(setConfig).toHaveBeenCalledWith('accelerator.connectionOverrides', [{ contextName: 'prod', enabled: false }]);
    confirm.mockRestore();
  });

  it('keeps custom artifact overrides visibly marked', () => {
    acceleratorConfig = { ...acceleratorConfig, customImage: 'ghcr.io/example/accelerator:test', customChart: 'ghcr.io/example/charts/accelerator:test' };
    acceleratorStatus = { state: 'active', enabled: true, namespace: 'default', available: true, versionMismatchWarning: true };
    render(<AcceleratorConnectionPanel contextName="prod" contextNamespace="default" onOpenSettings={vi.fn()} />);
    expect(screen.getByRole('alert').textContent).toContain('Custom Accelerator artifacts active');
    expect(screen.getByRole('alert').textContent).toContain('image');
    expect(screen.getByRole('alert').textContent).toContain('chart');
    expect(screen.getByRole('status').textContent).toContain('Connected despite an Accelerator version mismatch');
  });
});
