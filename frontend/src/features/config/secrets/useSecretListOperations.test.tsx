/** @vitest-environment jsdom */
import React from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SecretReadSource } from './secretReadSource';
import { SecretReadSourceProvider } from './secretReadSource';
import { useSecretListOperations } from './useSecretListOperations';
import Logger from '~/utils/Logger';

const k8s = vi.hoisted(() => ({
  lastRefresh: 1,
  reconcileToken: 1,
  checkConnectionError: vi.fn(() => false),
}));
vi.mock('~/context', () => ({ useK8s: () => k8s }));
vi.mock('wailsjs/go/main/App', () => ({
  CancelListRequest: vi.fn(), GetSecretData: vi.fn(), GetSecretYaml: vi.fn(), ListAcceleratorSecretsMetadata: vi.fn(), ListSecretsMetadata: vi.fn(),
  SubscribeResourceWatcher: vi.fn(), SubscribeSecretWatcher: vi.fn(), UnsubscribeSecretWatcher: vi.fn(), UnsubscribeWatcher: vi.fn(),
}));
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(() => () => {}) }));

class HookSource implements SecretReadSource {
  sourceKey: string;
  log: string[] = [];
  subscribeCount = 0;
  listCount = 0;
  unsubscribeCount = 0;
  subscribeError: unknown = null;
  cancelCount = 0;
  listeners = { progress: new Set<any>(), resource: new Set<any>(), status: new Set<any>(), error: new Set<any>(), connected: new Set<any>() };
  constructor(key: string) { this.sourceKey = key; }
  list = async (id: string, namespace: string) => {
    this.listCount += 1;
    this.log.push(`list:${namespace}:${id}`);
    return [{ metadata: { uid: `${this.sourceKey}:${namespace || 'all'}`, namespace, name: 'secret' }, type: 'Opaque', dataKeys: 1 }];
  };
  cancelList = async (id: string) => { this.cancelCount += 1; this.log.push(`cancel:${id}`); };
  subscribe = async (namespace: string, exclude: boolean) => {
    this.subscribeCount += 1;
    this.log.push(`subscribe:${namespace}:${exclude}`);
    if (this.subscribeError) throw this.subscribeError;
    return `spec:${this.sourceKey}:${namespace}:${exclude}`;
  };
  unsubscribe = async (id: string) => { this.unsubscribeCount += 1; this.log.push(`unsubscribe:${id}`); };
  getSecretData = async () => [];
  getSecretYaml = async () => '';
  private on(kind: keyof HookSource['listeners'], callback: any) { this.listeners[kind].add(callback); return () => { this.listeners[kind].delete(callback); this.log.push(`dispose:${kind}`); }; }
  onProgress = (callback: any) => this.on('progress', callback);
  onResource = (callback: any) => this.on('resource', callback);
  onStatus = (callback: any) => this.on('status', callback);
  onError = (callback: any) => this.on('error', callback);
  onConnected = (callback: any) => this.on('connected', callback);
}

describe('useSecretListOperations', () => {
  beforeEach(() => {
    k8s.lastRefresh = 1;
    k8s.reconcileToken = 1;
    k8s.checkConnectionError.mockClear();
  });

  it('keeps one controller across stable, option, visibility, refresh, and source transitions', async () => {
    const sourceA = new HookSource('source-a');
    const sourceB = new HookSource('source-b');
    let activeSource: SecretReadSource = sourceA;
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <SecretReadSourceProvider value={activeSource}>{children}</SecretReadSourceProvider>
    );
    const { result, rerender, unmount } = renderHook(
      (props: { selected: string[]; all: string[]; visible: boolean; context: string; hide: boolean }) =>
        useSecretListOperations(props.context, props.selected, props.all, props.visible, props.hide),
      { wrapper, initialProps: { selected: ['b', 'a'], all: ['a', 'b', 'c'], visible: true, context: 'ctx', hide: true } },
    );
    await waitFor(() => expect(result.current.secrets).toHaveLength(2));
    expect(sourceA.subscribeCount).toBe(2);

    rerender({ selected: ['a', 'b'], all: ['a', 'b', 'c'], visible: true, context: 'ctx', hide: true });
    await act(async () => { await Promise.resolve(); });
    expect(sourceA.subscribeCount).toBe(2);

    rerender({ selected: ['a', 'b'], all: ['a', 'b', 'c'], visible: true, context: 'ctx', hide: false });
    await waitFor(() => expect(sourceA.subscribeCount).toBe(4));
    const firstNewSubscribe = sourceA.log.findIndex(value => value === 'subscribe:a:false');
    const oldUnsubscribes = sourceA.log.map((value, index) => value.startsWith('unsubscribe:') ? index : -1).filter(index => index >= 0);
    const lastOldUnsubscribe = oldUnsubscribes[oldUnsubscribes.length - 1];
    expect(lastOldUnsubscribe).toBeLessThan(firstNewSubscribe);

    const subscribeBeforeRefresh = sourceA.subscribeCount;
    const listsBeforeRefresh = sourceA.listCount;
    k8s.lastRefresh += 1;
    rerender({ selected: ['a', 'b'], all: ['a', 'b', 'c'], visible: true, context: 'ctx', hide: false });
    await waitFor(() => expect(sourceA.listCount).toBeGreaterThan(listsBeforeRefresh));
    expect(sourceA.subscribeCount).toBe(subscribeBeforeRefresh);
    k8s.reconcileToken += 1;
    rerender({ selected: ['a', 'b'], all: ['a', 'b', 'c'], visible: true, context: 'ctx', hide: false });
    await waitFor(() => expect(sourceA.listCount).toBeGreaterThan(listsBeforeRefresh + 2));

    rerender({ selected: ['a', 'b'], all: ['a', 'b', 'c'], visible: false, context: 'ctx', hide: false });
    await waitFor(() => expect(result.current).toMatchObject({ secrets: [], loading: false, error: null }));
    const beforeHidden = sourceA.subscribeCount;
    rerender({ selected: ['a'], all: ['a', 'b', 'c'], visible: false, context: '', hide: false });
    await act(async () => { await Promise.resolve(); });
    expect(sourceA.subscribeCount).toBe(beforeHidden);

    activeSource = sourceB;
    rerender({ selected: ['*'], all: ['a', 'b', 'c'], visible: true, context: 'ctx', hide: true });
    await waitFor(() => expect(sourceB.subscribeCount).toBe(1));
    expect(sourceB.log[0]).toBe('subscribe::true');
    await waitFor(() => expect(result.current.secrets[0]?.metadata.uid).toBe('source-b:all'));
    unmount();
    await waitFor(() => expect(sourceB.unsubscribeCount).toBe(1));
    expect(sourceB.listeners.resource.size).toBe(0);
  });

  it('keeps the real desktop safe Logger diagnostic wired on subscription rejection', async () => {
    const source = new HookSource('source-error');
    source.subscribeError = { raw: 'HOSTILE_SECRET_VALUE' };
    const logger = vi.spyOn(Logger, 'error').mockImplementation(() => {});
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <SecretReadSourceProvider value={source}>{children}</SecretReadSourceProvider>
    );
    const mounted = renderHook(
      () => useSecretListOperations('ctx', ['team'], ['team', 'other'], true, true),
      { wrapper },
    );
    await waitFor(() => expect(logger).toHaveBeenCalledWith(
      'Secret list operation failure',
      expect.objectContaining({ event: 'subscription_failure', namespace: 'team' }),
      'config',
    ));
    expect(JSON.stringify(logger.mock.calls)).not.toContain('HOSTILE_SECRET_VALUE');
    mounted.unmount();
    logger.mockRestore();
  });
});
