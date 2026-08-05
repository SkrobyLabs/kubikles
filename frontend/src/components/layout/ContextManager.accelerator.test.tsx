/** @vitest-environment jsdom */
import React from 'react';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ContextManager from './ContextManager';

vi.mock('wailsjs/go/main/App', () => ({ GetContextDetails: vi.fn(() => new Promise(() => {})), DeleteContext: vi.fn(), RenameContext: vi.fn(), SelectKubeconfigFile: vi.fn() }));
vi.mock('~/context', () => ({
  useConfig: () => ({ config: { kubernetes: {}, accelerator: { connectionOverrides: [] } }, setConfig: vi.fn() }),
  useNotification: () => ({ addNotification: vi.fn() }),
}));
vi.mock('./ContextEditor', () => ({ default: ({ contextName, initialTab }: any) => <div data-testid="context-editor">{contextName}:{initialTab}</div> }));

describe('ContextManager Accelerator route', () => {
  it('starts directly in the requested context and Accelerator tab', () => {
    render(<ContextManager initialContext="prod" initialTab="accelerator" onClose={vi.fn()} onContextsChanged={vi.fn()} />);
    expect(screen.getByTestId('context-editor').textContent).toBe('prod:accelerator');
    expect(screen.queryByText('Context Manager')).toBeNull();
  });
});
