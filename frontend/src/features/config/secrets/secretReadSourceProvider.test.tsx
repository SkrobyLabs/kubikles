/** @vitest-environment jsdom */
import React, { StrictMode, useEffect } from 'react';
import { act, render, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const bridge = vi.hoisted(() => ({
  CancelIntegratedSecretListRequest: vi.fn(), CancelListRequest: vi.fn(),
  GetIntegratedSecretData: vi.fn(), GetIntegratedSecretYaml: vi.fn(), GetSecretData: vi.fn(), GetSecretYaml: vi.fn(),
  ListAcceleratorSecretsMetadata: vi.fn(), ListIntegratedSecretsMetadata: vi.fn(), ListSecretsMetadata: vi.fn(),
  ReleaseIntegratedSecretReads: vi.fn(), RetainIntegratedSecretReads: vi.fn(),
  SubscribeIntegratedSecretWatcher: vi.fn(), SubscribeResourceWatcher: vi.fn(), SubscribeSecretWatcher: vi.fn(),
  UnsubscribeIntegratedSecretWatcher: vi.fn(), UnsubscribeSecretWatcher: vi.fn(), UnsubscribeWatcher: vi.fn(),
}));
const runtime = vi.hoisted(() => {
  const listeners = new Map<string, Set<(value: any) => void>>();
  return {
    listeners,
    EventsOn: vi.fn((name: string, callback: (value: any) => void) => {
      if (!listeners.has(name)) listeners.set(name, new Set());
      listeners.get(name)!.add(callback);
      return () => listeners.get(name)?.delete(callback);
    }),
    emit(name: string, value: any) { for (const callback of [...(listeners.get(name) ?? [])]) callback(value); },
  };
});
const mode = vi.hoisted(() => ({ server: false }));

vi.mock('wailsjs/go/main/App', () => bridge);
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: runtime.EventsOn }));
vi.mock('~/lib/wailsjs-adapter/runtime/runtime', () => ({ EventsOn: runtime.EventsOn, isInServerMode: () => mode.server }));

import { IntegratedSecretReadSourceProvider, RuntimeSecretReadSourceProvider, useSecretReadSource } from './secretReadSource';
import { SecretListOperationController } from './secretListOperations';

const token = (letter: string, generation: number) => `s.${letter.repeat(22)}.${generation.toString(16).padStart(16, '0')}`;
const observedSource = vi.fn();

function Consumer() {
  const source = useSecretReadSource();
  useEffect(() => observedSource(source.sourceKey), [source]);
  return <span>Secret consumer</span>;
}

function ControllerConsumer({ calls }: { calls: string[] }) {
  const source = useSecretReadSource();
  const controller = React.useMemo(() => new SecretListOperationController(source, () => {}), []);
  useEffect(() => {
    calls.push(source.sourceKey);
    void controller.replace(source, [], false, false);
  }, [calls, controller, source]);
  useEffect(() => () => { void controller.stop(); }, [controller]);
  return <span>Controller consumer</span>;
}

function ServerSecretView() {
  const source = useSecretReadSource();
  useEffect(() => { void source.list('server-navigation', '', true); }, [source]);
  return <span>Server Secrets</span>;
}

const currentSource = () => observedSource.mock.calls[observedSource.mock.calls.length - 1]?.[0];

describe('IntegratedSecretReadSourceProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    observedSource.mockClear();
    runtime.listeners.clear();
    mode.server = false;
    bridge.RetainIntegratedSecretReads.mockImplementation(async () => {
      expect(runtime.listeners.get('accelerator:secret-source-ready')?.size).toBe(1);
      expect(runtime.listeners.get('accelerator:secret-source-unavailable')?.size).toBe(1);
    });
    bridge.ReleaseIntegratedSecretReads.mockResolvedValue(undefined);
  });

  afterEach(() => vi.restoreAllMocks());

  it('starts Direct and retains only one exact opaque control token', async () => {
    const mounted = render(<IntegratedSecretReadSourceProvider><Consumer /></IntegratedSecretReadSourceProvider>);
    expect(currentSource()).toBe('direct-secrets');
    await waitFor(() => expect(bridge.RetainIntegratedSecretReads).toHaveBeenCalledOnce());

    const first = token('A', 1);
    const second = token('B', 2);
    act(() => runtime.emit('accelerator:secret-source-ready', { sourceToken: first, reason: 'identity-leak' }));
    expect(currentSource()).toBe('direct-secrets');
    act(() => runtime.emit('accelerator:secret-source-ready', { sourceToken: first }));
    expect(currentSource()).toBe(first);
    act(() => runtime.emit('accelerator:secret-source-ready', { sourceToken: first }));
    expect(currentSource()).toBe(first);
    act(() => runtime.emit('accelerator:secret-source-ready', { sourceToken: second }));
    expect(currentSource()).toBe(first);
    act(() => runtime.emit('accelerator:secret-source-unavailable', { sourceToken: second }));
    expect(currentSource()).toBe(first);
    act(() => runtime.emit('accelerator:secret-source-unavailable', { sourceToken: first }));
    expect(currentSource()).toBe('direct-secrets');
    expect(mounted.container.textContent).toBe('Secret consumer');
    expect(mounted.container.textContent).not.toContain(first);
    expect(mounted.container.textContent).not.toContain(second);
    mounted.unmount();
    await waitFor(() => expect(bridge.ReleaseIntegratedSecretReads).toHaveBeenCalledOnce());
  });

  it('retains the backend route while navigating away from Secret consumers', async () => {
    const mounted = render(<IntegratedSecretReadSourceProvider><Consumer /></IntegratedSecretReadSourceProvider>);
    await waitFor(() => expect(bridge.RetainIntegratedSecretReads).toHaveBeenCalledOnce());

    mounted.rerender(<IntegratedSecretReadSourceProvider><span>Other view</span></IntegratedSecretReadSourceProvider>);
    expect(bridge.ReleaseIntegratedSecretReads).not.toHaveBeenCalled();

    mounted.rerender(<IntegratedSecretReadSourceProvider><Consumer /></IntegratedSecretReadSourceProvider>);
    expect(bridge.RetainIntegratedSecretReads).toHaveBeenCalledOnce();

    mounted.unmount();
    await waitFor(() => expect(bridge.ReleaseIntegratedSecretReads).toHaveBeenCalledOnce());
  });

  it.each([false, true])('commits Direct before a batched replacement ready (StrictMode=%s)', async strict => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const consoleSpies = [
      vi.spyOn(console, 'log'),
      vi.spyOn(console, 'info'),
      vi.spyOn(console, 'warn'),
      vi.spyOn(console, 'error'),
      vi.spyOn(console, 'debug'),
    ];
    const controllerCalls: string[] = [];
    const content = <IntegratedSecretReadSourceProvider><ControllerConsumer calls={controllerCalls} /></IntegratedSecretReadSourceProvider>;
    const mounted = render(strict ? <StrictMode>{content}</StrictMode> : content);
    const first = token('A', 1);
    const second = token('B', 2);
    const ignored = token('C', 3);
    act(() => runtime.emit('accelerator:secret-source-ready', { sourceToken: first }));
    await waitFor(() => expect(controllerCalls[controllerCalls.length - 1]).toBe(first));
    const firstIndex = controllerCalls.lastIndexOf(first);

    act(() => {
      runtime.emit('accelerator:secret-source-unavailable', { sourceToken: first });
      runtime.emit('accelerator:secret-source-ready', { sourceToken: second });
      runtime.emit('accelerator:secret-source-ready', { sourceToken: ignored });
    });

    await waitFor(() => expect(controllerCalls[controllerCalls.length - 1]).toBe(second));
    expect(controllerCalls.slice(firstIndex)).toEqual([first, 'direct-secrets', second]);
    expect(controllerCalls).not.toContain(ignored);
    expect(mounted.container.textContent).toBe('Controller consumer');
    expect(mounted.container.textContent).not.toMatch(/s\.[A-Za-z0-9_-]{22}\.[0-9a-f]{16}/);
    mounted.unmount();
    expect(storageSet).not.toHaveBeenCalled();
    expect(JSON.stringify(consoleSpies.flatMap(spy => spy.mock.calls))).not.toMatch(/s\.[A-Za-z0-9_-]{22}\.[0-9a-f]{16}/);
  });

  it('keeps Direct when the sole pending token becomes unavailable', async () => {
    const controllerCalls: string[] = [];
    const mounted = render(<IntegratedSecretReadSourceProvider><ControllerConsumer calls={controllerCalls} /></IntegratedSecretReadSourceProvider>);
    const first = token('A', 1);
    const pending = token('B', 2);
    act(() => runtime.emit('accelerator:secret-source-ready', { sourceToken: first }));
    await waitFor(() => expect(controllerCalls[controllerCalls.length - 1]).toBe(first));
    const firstIndex = controllerCalls.lastIndexOf(first);

    act(() => {
      runtime.emit('accelerator:secret-source-unavailable', { sourceToken: first });
      runtime.emit('accelerator:secret-source-ready', { sourceToken: pending });
      runtime.emit('accelerator:secret-source-unavailable', { sourceToken: pending });
    });

    await waitFor(() => expect(controllerCalls[controllerCalls.length - 1]).toBe('direct-secrets'));
    expect(controllerCalls.slice(firstIndex)).toEqual([first, 'direct-secrets']);
    expect(controllerCalls).not.toContain(pending);
    mounted.unmount();
  });

  it('serializes StrictMode retain and release into exact successful pairs', async () => {
    const mounted = render(<StrictMode><IntegratedSecretReadSourceProvider><Consumer /></IntegratedSecretReadSourceProvider></StrictMode>);
    await waitFor(() => expect(bridge.RetainIntegratedSecretReads.mock.calls.length - bridge.ReleaseIntegratedSecretReads.mock.calls.length).toBe(1));
    mounted.unmount();
    await waitFor(() => expect(bridge.ReleaseIntegratedSecretReads).toHaveBeenCalledTimes(bridge.RetainIntegratedSecretReads.mock.calls.length));
  });

  it('keeps ordinary server Secret mount and navigation Direct without integrated controls', async () => {
    mode.server = true;
    bridge.ListSecretsMetadata.mockResolvedValue([]);
    const mounted = render(<RuntimeSecretReadSourceProvider><ServerSecretView /></RuntimeSecretReadSourceProvider>);
    mounted.rerender(<RuntimeSecretReadSourceProvider><span>Other view</span></RuntimeSecretReadSourceProvider>);
    mounted.rerender(<RuntimeSecretReadSourceProvider><ServerSecretView /></RuntimeSecretReadSourceProvider>);
    await waitFor(() => expect(bridge.ListSecretsMetadata).toHaveBeenCalledTimes(2));
    expect(bridge.ListSecretsMetadata).toHaveBeenNthCalledWith(1, 'server-navigation', '');
    expect(bridge.RetainIntegratedSecretReads).not.toHaveBeenCalled();
    expect(bridge.ReleaseIntegratedSecretReads).not.toHaveBeenCalled();
    for (const call of [
      bridge.ListIntegratedSecretsMetadata, bridge.GetIntegratedSecretData, bridge.GetIntegratedSecretYaml,
      bridge.CancelIntegratedSecretListRequest, bridge.SubscribeIntegratedSecretWatcher, bridge.UnsubscribeIntegratedSecretWatcher,
    ]) expect(call).not.toHaveBeenCalled();
    expect(runtime.EventsOn).not.toHaveBeenCalled();
    expect(runtime.listeners.size).toBe(0);
    mounted.unmount();
  });
});
