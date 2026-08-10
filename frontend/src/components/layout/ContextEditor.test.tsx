/** @vitest-environment jsdom */
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ContextEditor from './ContextEditor';

vi.mock('wailsjs/go/main/App', () => ({
  GetFullContextDetail: vi.fn(async () => ({
    name: 'prod',
    cluster: 'prod-cluster',
    authInfo: 'prod-user',
    namespace: 'default',
    isActive: false,
    clusterDetail: {},
    authDetail: {},
  })),
  UpdateContextDetail: vi.fn(),
}));

vi.mock('~/context', () => ({
  useK8s: () => ({ retryConnection: vi.fn(), triggerRefresh: vi.fn() }),
  useNotification: () => ({ addNotification: vi.fn() }),
}));

vi.mock('~/features/accelerator/AcceleratorConnectionPanel', () => ({ default: () => null }));

describe('ContextEditor', () => {
  it('shows a top-right close button that closes the dialog', async () => {
    const onClose = vi.fn();
    render(
      <ContextEditor
        contextName="prod"
        onBack={vi.fn()}
        onClose={onClose}
        onSaved={vi.fn()}
        onOpenAcceleratorSettings={vi.fn()}
      />,
    );

    fireEvent.click(await screen.findByRole('button', { name: 'Close context editor' }));
    expect(onClose).toHaveBeenCalledOnce();
  });
});
