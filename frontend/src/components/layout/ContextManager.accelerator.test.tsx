/** @vitest-environment jsdom */
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import ContextManager from './ContextManager';

vi.mock('wailsjs/go/main/App', () => ({ GetContextDetails: vi.fn(() => new Promise(() => {})), DeleteContext: vi.fn(), RenameContext: vi.fn(), SelectKubeconfigFile: vi.fn() }));
const openConfigEditor = vi.fn();
vi.mock('~/context', () => ({
  useConfig: () => ({ config: { kubernetes: {}, accelerator: { connectionOverrides: [] } }, setConfig: vi.fn(), openConfigEditor }),
  useNotification: () => ({ addNotification: vi.fn() }),
}));
vi.mock('./ContextEditor', () => ({ default: ({ contextName, initialTab, onClose, onOpenAcceleratorSettings }: any) => <div data-testid="context-editor"><span data-testid="context-route">{contextName}:{initialTab}</span><button onClick={onClose}>Close editor</button><button onClick={onOpenAcceleratorSettings}>Open settings</button></div> }));

describe('ContextManager Accelerator route', () => {
  it('closes the context list when Escape is pressed', () => {
    const onClose = vi.fn();
    render(<ContextManager onClose={onClose} onContextsChanged={vi.fn()} />);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('closes the context editor when Escape is pressed', () => {
    const onClose = vi.fn();
    render(<ContextManager initialContext="prod" onClose={onClose} onContextsChanged={vi.fn()} />);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('allows the context editor to close the entire dialog', () => {
    const onClose = vi.fn();
    render(<ContextManager initialContext="prod" onClose={onClose} onContextsChanged={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'Close editor' }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('starts directly in the requested context and Accelerator tab', () => {
    render(<ContextManager initialContext="prod" initialTab="accelerator" onClose={vi.fn()} onContextsChanged={vi.fn()} />);
    expect(screen.getByTestId('context-route').textContent).toBe('prod:accelerator');
    expect(screen.queryByText('Context Manager')).toBeNull();
  });

  it('closes Context Manager before opening Accelerator settings', () => {
    const onClose = vi.fn();
    render(<ContextManager initialContext="prod" initialTab="accelerator" onClose={onClose} onContextsChanged={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'Open settings' }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(openConfigEditor).toHaveBeenCalledWith('accelerator');
    expect(onClose.mock.invocationCallOrder[0]).toBeLessThan(openConfigEditor.mock.invocationCallOrder[0]);
  });
});
