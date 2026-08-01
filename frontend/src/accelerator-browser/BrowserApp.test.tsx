/** @vitest-environment jsdom */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { StrictMode, useLayoutEffect } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';
import BrowserApp from './BrowserApp';
import { createTestFacade } from './testFacade';

const rows = [
  { metadata: { name: 'older', namespace: 'zeta', uid: 'uid-z', creationTimestamp: '2025-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 0 },
  { metadata: { name: 'newer', namespace: 'alpha', uid: 'uid-a', creationTimestamp: '2026-01-01T00:00:00Z' }, type: 'kubernetes.io/tls', dataKeys: 2 },
];
const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(next => { resolve = next; });
  return { promise, resolve };
};

describe('Accelerator Browser app', () => {
  it('attaches facade events before first subscribe and buffers its immediate owned event', async () => {
    const test = createTestFacade();
    const calls: string[] = [];
    (test.facade.events.subscribe as any).mockImplementation((callback: any) => {
      calls.push('attach');
      test.subscribers.add(callback);
      return () => test.subscribers.delete(callback);
    });
    (test.facade.SubscribeSecretWatcher as any).mockImplementation(async () => {
      calls.push('subscribe');
      test.emit({ type: 'resource-event', data: {
        type: 'ADDED', resourceType: 'secrets', namespace: 'instant', watcherSpecId: 'spec-all',
        resource: { metadata: { name: 'instant-secret', namespace: 'instant', uid: 'instant-uid', creationTimestamp: '2026-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 1 },
      } });
      return { watcherSpecId: 'spec-all' };
    });
    (test.facade.ListSecretsMetadata as any).mockResolvedValue([]);
    render(<BrowserApp facade={test.facade} />);
    expect(await screen.findByText('instant-secret')).toBeTruthy();
    expect(calls.slice(0, 2)).toEqual(['attach', 'subscribe']);
  });

  it('shows the fixed Secret-only list and owns namespace and Hide Helm transitions', async () => {
    const test = createTestFacade();
    (test.facade.ListSecretsMetadata as any).mockResolvedValue(rows);
    render(<BrowserApp facade={test.facade} />);
    await screen.findByText('newer');
    expect(test.facade.SubscribeSecretWatcher).toHaveBeenCalledWith('', true);
    expect(test.facade.ListSecretsMetadata).toHaveBeenCalledWith(expect.stringMatching(/^secret-/), '', true);
    expect(screen.getAllByRole('columnheader').map(value => value.textContent)).toEqual(['Name', 'Namespace', 'Type', 'Age', 'Keys']);
    expect(screen.getByText('2 keys')).toBeTruthy();
    expect(screen.getByText('-')).toBeTruthy();
    await waitFor(() => expect(screen.getByRole('option', { name: 'alpha' })).toBeTruthy());
    fireEvent.change(screen.getByLabelText('Namespace'), { target: { value: 'alpha' } });
    await waitFor(() => expect(test.facade.SubscribeSecretWatcher).toHaveBeenCalledWith('alpha', true));
    fireEvent.click(screen.getByLabelText('Hide Helm'));
    await waitFor(() => expect(test.facade.SubscribeSecretWatcher).toHaveBeenCalledWith('alpha', false));
    fireEvent.change(screen.getByLabelText('Search Secrets'), { target: { value: 'tls' } });
    expect(screen.queryByText('older')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Open newer' }));
    await screen.findByRole('button', { name: 'Back' });
    expect((screen.getByLabelText('Namespace') as HTMLSelectElement).value).toBe('alpha');
    expect((screen.getByLabelText('Search Secrets') as HTMLInputElement).value).toBe('tls');
    fireEvent.click(screen.getByRole('button', { name: 'Back' }));
    expect((screen.getByLabelText('Namespace') as HTMLSelectElement).value).toBe('alpha');
    expect(screen.getAllByRole('checkbox')).toHaveLength(1);
    expect(document.body.innerHTML).not.toMatch(/sidebar|menu|palette|Settings|Create|bulk|Actions/i);
  });

  it('clears sensitive state synchronously and remains terminal after late work', async () => {
    const test = createTestFacade();
    (test.facade.ListSecretsMetadata as any).mockResolvedValue(rows.slice(0, 1));
    (test.facade.GetSecretYaml as any).mockResolvedValue('HOSTILE_YAML');
    (test.facade.GetSecretData as any).mockResolvedValue([{ key: 'HOSTILE_KEY', value: 'HOSTILE_VALUE', base64Value: 'SE9TVElMRQ==', isBinary: false, source: 'data', encoding: 'text' }]);
    render(<BrowserApp facade={test.facade} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Open older' }));
    await screen.findByRole('button', { name: 'Back' });
    await screen.findByText('SE9TVElMRQ==');
    act(() => test.emit({ type: 'terminal' }));
    expect(document.body.textContent).toBe('Reopen from Kubikles');
    expect(document.body.innerHTML).not.toMatch(/older|zeta|HOSTILE|uid-z/);
    await waitFor(() => expect(test.facade.UnsubscribeSecretWatcher).toHaveBeenCalled());
    await waitFor(() => expect(test.facade.close).toHaveBeenCalledOnce());
    act(() => test.emit({ type: 'connected', data: { resumed: true } }));
    expect(document.body.textContent).toBe('Reopen from Kubikles');
  });

  it('applies owned live events and reconciles exact gap signals without resubscribing', async () => {
    const test = createTestFacade();
    (test.facade.ListSecretsMetadata as any).mockResolvedValue([]);
    render(<BrowserApp facade={test.facade} />);
    await waitFor(() => expect(test.facade.ListSecretsMetadata).toHaveBeenCalledOnce());
    act(() => test.emit({ type: 'resource-event', data: {
      type: 'ADDED', resourceType: 'secrets', namespace: 'live', watcherSpecId: 'spec-all',
      resource: { metadata: { name: 'live-secret', namespace: 'live', uid: 'live-uid', creationTimestamp: '2026-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 1 },
    } }));
    expect(await screen.findByText('live-secret')).toBeTruthy();
    act(() => test.emit({ type: 'resource-event', data: {
      type: 'DELETED', resourceType: 'secrets', namespace: 'live', watcherSpecId: 'spec-all',
      resource: { metadata: { name: 'live-secret', namespace: 'live', uid: 'live-uid', creationTimestamp: '2026-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 1 },
    } }));
    await waitFor(() => expect(screen.queryByText('live-secret')).toBeNull());
    expect(screen.getByRole('option', { name: 'live' })).toBeTruthy();
    const subscribeCount = (test.facade.SubscribeSecretWatcher as any).mock.calls.length;
    act(() => test.emit({ type: 'watcher-error', data: { watcherSpecId: 'spec-all', code: 'resource_version_expired', recoverable: true } }));
    await waitFor(() => expect(test.facade.ListSecretsMetadata).toHaveBeenCalledTimes(2));
    expect(test.facade.SubscribeSecretWatcher).toHaveBeenCalledTimes(subscribeCount);
  });

  it('keeps an only discovered named namespace exact and reports safe partial failures', async () => {
    const test = createTestFacade();
    (test.facade.ListSecretsMetadata as any).mockResolvedValue([rows[0]]);
    render(<BrowserApp facade={test.facade} />);
    await screen.findByText('older');
    fireEvent.change(screen.getByLabelText('Namespace'), { target: { value: 'zeta' } });
    await waitFor(() => expect(test.facade.ListSecretsMetadata).toHaveBeenCalledWith(expect.stringMatching(/^secret-/), 'zeta', true));
    (test.facade.SubscribeSecretWatcher as any).mockRejectedValueOnce({ raw: 'HOSTILE_SECRET_VALUE' });
    fireEvent.click(screen.getByLabelText('Hide Helm'));
    await screen.findByText('Some Secrets could not be loaded');
    expect(document.body.innerHTML).not.toContain('HOSTILE_SECRET_VALUE');
    await waitFor(() => expect(test.facade.ListSecretsMetadata).toHaveBeenCalledWith(expect.stringMatching(/^secret-/), 'zeta', false));
  });

  it('renders terminal synchronously then orders cancel, unsubscribe, detach, and close exactly once', async () => {
    const test = createTestFacade();
    const calls: string[] = [];
    const list = deferred<any[]>();
    const cancel = deferred<void>();
    (test.facade.ListSecretsMetadata as any).mockReturnValue(list.promise);
    (test.facade.CancelListRequest as any).mockImplementation(() => { calls.push('cancel'); return cancel.promise; });
    (test.facade.UnsubscribeSecretWatcher as any).mockImplementation(async () => { calls.push('unsubscribe'); });
    (test.facade.events.subscribe as any).mockImplementation((callback: any) => {
      test.subscribers.add(callback);
      return () => { calls.push('detach'); test.subscribers.delete(callback); };
    });
    (test.facade.close as any).mockImplementation(async () => { calls.push('close'); });
    render(<BrowserApp facade={test.facade} />);
    await waitFor(() => expect(test.facade.ListSecretsMetadata).toHaveBeenCalledOnce());
    act(() => test.emit({ type: 'terminal' }));
    expect(document.body.textContent).toBe('Reopen from Kubikles');
    expect(calls).not.toContain('detach');
    expect(calls).not.toContain('close');
    await waitFor(() => expect(calls).toEqual(['cancel']));
    cancel.resolve();
    await waitFor(() => expect(calls).toEqual(['cancel', 'unsubscribe', 'detach', 'close']));
    list.resolve(rows);
    act(() => test.emit({ type: 'resource-event', data: { type: 'ADDED', resourceType: 'secrets', namespace: 'late', watcherSpecId: 'spec-all', resource: rows[0] } }));
    expect(document.body.textContent).toBe('Reopen from Kubikles');
    expect(test.facade.close).toHaveBeenCalledOnce();
  });

  it('settles a late subscription with exact unsubscribe before close and never starts its list', async () => {
    const test = createTestFacade();
    const subscribe = deferred<any>();
    const calls: string[] = [];
    (test.facade.SubscribeSecretWatcher as any).mockReturnValue(subscribe.promise);
    (test.facade.UnsubscribeSecretWatcher as any).mockImplementation(async () => { calls.push('unsubscribe'); });
    (test.facade.events.subscribe as any).mockImplementation((callback: any) => {
      test.subscribers.add(callback);
      return () => { calls.push('detach'); test.subscribers.delete(callback); };
    });
    (test.facade.close as any).mockImplementation(async () => { calls.push('close'); });
    render(<BrowserApp facade={test.facade} />);
    await waitFor(() => expect(test.facade.SubscribeSecretWatcher).toHaveBeenCalledOnce());
    act(() => test.emit({ type: 'terminal' }));
    subscribe.resolve({ watcherSpecId: 'late-spec' });
    await waitFor(() => expect(calls).toEqual(['unsubscribe', 'detach', 'close']));
    expect(test.facade.ListSecretsMetadata).not.toHaveBeenCalled();
    expect(test.facade.UnsubscribeSecretWatcher).toHaveBeenCalledWith('late-spec');
  });

  it('attaches and detaches deterministically through StrictMode cleanup', async () => {
    const test = createTestFacade();
    (test.facade.ListSecretsMetadata as any).mockResolvedValue([]);
    const mounted = render(<StrictMode><BrowserApp facade={test.facade} /></StrictMode>);
    await waitFor(() => expect(test.subscribers.size).toBe(1));
    mounted.unmount();
    await waitFor(() => expect(test.subscribers.size).toBe(0));
    expect(test.facade.close).toHaveBeenCalledOnce();
  });

  it('fences a sibling layout terminal in StrictMode before any Secret work starts', async () => {
    const test = createTestFacade();
    const host = document.createElement('div');
    document.body.append(host);
    const root = createRoot(host);
    function SiblingTerminal() {
      useLayoutEffect(() => { test.emit({ type: 'terminal' }); }, []);
      return null;
    }
    act(() => root.render(<StrictMode><BrowserApp facade={test.facade} /><SiblingTerminal /></StrictMode>));
    expect(host.textContent).toBe('Reopen from Kubikles');
    expect(test.facade.SubscribeSecretWatcher).not.toHaveBeenCalled();
    expect(test.facade.ListSecretsMetadata).not.toHaveBeenCalled();
    expect(test.facade.GetSecretData).not.toHaveBeenCalled();
    expect(test.facade.GetSecretYaml).not.toHaveBeenCalled();
    await waitFor(() => expect(test.subscribers.size).toBe(0));
    expect(test.facade.close).toHaveBeenCalledOnce();
    act(() => root.unmount());
    host.remove();
  });

  it('fences a terminal delivered synchronously inside layout attachment', async () => {
    const test = createTestFacade();
    const detach = vi.fn();
    (test.facade.events.subscribe as any).mockImplementation((callback: any) => {
      callback({ type: 'terminal' });
      return detach;
    });
    render(<BrowserApp facade={test.facade} />);
    expect(document.body.textContent).toBe('Reopen from Kubikles');
    expect(test.facade.SubscribeSecretWatcher).not.toHaveBeenCalled();
    expect(test.facade.ListSecretsMetadata).not.toHaveBeenCalled();
    expect(detach).toHaveBeenCalledOnce();
    await waitFor(() => expect(test.facade.close).toHaveBeenCalledOnce());
  });
});
