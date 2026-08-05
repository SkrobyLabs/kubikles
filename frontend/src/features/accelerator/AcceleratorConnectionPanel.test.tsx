/** @vitest-environment jsdom */
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AcceleratorConnectionPanel from './AcceleratorConnectionPanel';

vi.mock('~/context', () => ({
  useConfig: () => ({ config: { accelerator: { enabledByDefault: false, defaultNamespace: '', connectionOverrides: [] } }, setConfig: vi.fn() }),
  useK8s: () => ({ currentContext: 'prod', setSelectedNamespaces: vi.fn() }),
  useUI: () => ({ navigateWithSearch: vi.fn() }),
  useAccelerator: () => ({ status: { state: 'direct_only', enabled: false, namespace: '', available: false }, enable: vi.fn(), retry: vi.fn(), disable: vi.fn() }),
}));

describe('AcceleratorConnectionPanel', () => {
  it('links inheritance guidance to global Accelerator settings', () => {
    const onOpenSettings = vi.fn();
    render(<AcceleratorConnectionPanel contextName="prod" contextNamespace="default" onOpenSettings={onOpenSettings} />);
    expect(screen.queryByText('Enabled by')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Open Settings > Accelerator' }));
    expect(onOpenSettings).toHaveBeenCalledOnce();
  });
});
