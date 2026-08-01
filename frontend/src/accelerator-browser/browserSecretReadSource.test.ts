import { describe, expect, it, vi } from 'vitest';
import { createBrowserSecretReadSource } from './browserSecretReadSource';
import { isAcceleratorBrowserFacade } from './facade';
import { createTestFacade } from './testFacade';

const summary = (uid = 'uid-one') => ({ metadata: { name: 'one', namespace: 'team', uid, creationTimestamp: '2026-01-01T00:00:00Z', labels: { hostile: 'HOSTILE' } }, type: 'Opaque', dataKeys: 1, data: { password: 'HOSTILE_VALUE' } });

describe('Browser Secret read source', () => {
  it('forwards the exact six operations and projects hostile results', async () => {
    const test = createTestFacade();
    (test.facade.ListSecretsMetadata as any).mockResolvedValue([summary()]);
    (test.facade.GetSecretData as any).mockResolvedValue([{ key: 'token', value: 'plain', base64Value: 'cGxhaW4=', isBinary: false, source: 'data', encoding: 'text', hostile: 'DROP' }]);
    (test.facade.GetSecretYaml as any).mockResolvedValue('kind: Secret');
    const handle = createBrowserSecretReadSource(test.facade);
    handle.attach();
    expect(handle.source.sourceKey).toBe('accelerator-browser');
    await expect(handle.source.list('request', 'team', true)).resolves.toEqual([{
      metadata: { name: 'one', namespace: 'team', uid: 'uid-one', creationTimestamp: '2026-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 1,
    }]);
    await handle.source.cancelList('request');
    await expect(handle.source.subscribe('team', true)).resolves.toBe('spec-all');
    await handle.source.unsubscribe('spec-all');
    await expect(handle.source.getSecretData('team', 'one')).resolves.toEqual([{ key: 'token', value: 'plain', base64Value: 'cGxhaW4=', isBinary: false, source: 'data', encoding: 'text' }]);
    await expect(handle.source.getSecretYaml('team', 'one')).resolves.toBe('kind: Secret');
    expect(test.facade.ListSecretsMetadata).toHaveBeenCalledWith('request', 'team', true);
    expect(test.facade.CancelListRequest).toHaveBeenCalledWith('request');
    expect(test.facade.SubscribeSecretWatcher).toHaveBeenCalledWith('team', true);
    expect(test.facade.UnsubscribeSecretWatcher).toHaveBeenCalledWith('spec-all');
    expect(test.facade.GetSecretData).toHaveBeenCalledWith('team', 'one');
    expect(test.facade.GetSecretYaml).toHaveBeenCalledWith('team', 'one');
    handle.dispose();
  });

  it('maps only owned value-free events and fences terminal work', async () => {
    const test = createTestFacade();
    let resolveSubscribe!: (value: any) => void;
    (test.facade.SubscribeSecretWatcher as any).mockReturnValue(new Promise(resolve => { resolveSubscribe = resolve; }));
    const handle = createBrowserSecretReadSource(test.facade);
    const resources: any[] = [];
    const statuses: any[] = [];
    const terminal = vi.fn();
    handle.source.onResource(event => resources.push(event));
    handle.source.onStatus(event => statuses.push(event));
    handle.onTerminal(terminal);
    handle.attach();
    const subscription = handle.source.subscribe('team', false);
    test.emit({ type: 'resource-event', data: { type: 'ADDED', resourceType: 'secrets', namespace: 'team', watcherSpecId: 'owned', resource: summary() as any } });
    resolveSubscribe({ watcherSpecId: 'owned' });
    await subscription;
    expect(resources).toHaveLength(1);
    expect(JSON.stringify(resources)).not.toMatch(/HOSTILE|labels|password|"data":/);
    test.emit({ type: 'watcher-status', data: { watcherSpecId: 'wrong', status: 'reconnecting' } });
    test.emit({ type: 'watcher-status', data: { watcherSpecId: 'owned', status: 'reconnecting' } });
    expect(statuses).toHaveLength(1);
    test.emit({ type: 'terminal' });
    test.emit({ type: 'terminal' });
    expect(terminal).toHaveBeenCalledOnce();
    await expect(handle.source.list('late', '', true)).rejects.toThrow('unavailable');
    await handle.source.cancelList('cleanup');
    await handle.source.unsubscribe('owned');
    handle.dispose();
    expect(test.subscribers.size).toBe(0);
  });

  it('is render-pure and latches a synchronous terminal for exact late replay', () => {
    const test = createTestFacade();
    (test.facade.events.subscribe as any).mockImplementation((callback: any) => {
      callback({ type: 'terminal' });
      return vi.fn();
    });
    const handle = createBrowserSecretReadSource(test.facade);
    expect(test.facade.events.subscribe).not.toHaveBeenCalled();
    const first = vi.fn();
    handle.onTerminal(first);
    expect(handle.attach()).toBe(false);
    expect(first).toHaveBeenCalledOnce();
    const late = vi.fn();
    handle.onTerminal(late);
    expect(late).toHaveBeenCalledOnce();
    handle.dispose();
  });

  it('rejects non-array list/detail shapes and ignores malformed exact events', async () => {
    const test = createTestFacade();
    const handle = createBrowserSecretReadSource(test.facade);
    const progress = vi.fn();
    const resources = vi.fn();
    handle.source.onProgress(progress);
    handle.source.onResource(resources);
    handle.attach();
    (test.facade.ListSecretsMetadata as any).mockResolvedValue(null);
    (test.facade.GetSecretData as any).mockResolvedValue({ raw: 'HOSTILE' });
    (test.facade.GetSecretYaml as any).mockResolvedValue(42);
    await expect(handle.source.list('request', '', true)).rejects.toThrow('Unable to load Secrets');
    await expect(handle.source.getSecretData('team', 'one')).rejects.toThrow('Unable to load Secret detail');
    await expect(handle.source.getSecretYaml('team', 'one')).rejects.toThrow('Unable to load Secret detail');
    for (const event of [
      null,
      { type: 'terminal', data: {} },
      { type: 'list-progress', data: { resourceType: 'secrets', requestId: 'r', loaded: -1, total: 1 } },
      { type: 'list-progress', data: { resourceType: 'secrets', requestId: 'r', loaded: '1', total: 1 } },
      { type: 'list-progress', data: { resourceType: 'secrets', requestId: 'r', loaded: 1, total: Number.NaN } },
      { type: 'connected', data: { resumed: 1 } },
      { type: 'unknown', data: {} },
    ]) test.emit(event as any);
    expect(progress).not.toHaveBeenCalled();
    expect(resources).not.toHaveBeenCalled();
    handle.dispose();
  });

  it('requires the exact closed facade and events own-key shapes', () => {
    const valid = createTestFacade().facade;
    expect(isAcceleratorBrowserFacade(valid)).toBe(true);
    const nullEvents = Object.assign(Object.create(null), valid.events);
    const nullFacade = Object.assign(Object.create(null), valid, { events: nullEvents });
    expect(isAcceleratorBrowserFacade(nullFacade)).toBe(true);
    const inheritedFacade = Object.assign(Object.create({ bearer: 'HOSTILE' }), valid);
    const inheritedSymbolFacade = Object.assign(Object.create({ [Symbol('socket')]: 'HOSTILE' }), valid);
    const inheritedEvents = { ...valid, events: Object.assign(Object.create({ routing: 'HOSTILE' }), valid.events) };
    const inheritedSymbolEvents = { ...valid, events: Object.assign(Object.create({ [Symbol('credential')]: 'HOSTILE' }), valid.events) };
    for (const candidate of [
      Object.create(valid),
      inheritedFacade,
      inheritedSymbolFacade,
      inheritedEvents,
      inheritedSymbolEvents,
      { ...valid, bearer: 'HOSTILE' },
      { ...valid, events: { ...valid.events, socket: 'HOSTILE' } },
      { ...valid, close: undefined },
      Object.assign({ [Symbol('HOSTILE')]: true }, valid),
    ]) expect(isAcceleratorBrowserFacade(candidate)).toBe(false);
    const getter = vi.fn();
    const accessor = { ...valid } as any;
    Object.defineProperty(accessor, 'GetSecretYaml', { enumerable: true, get: getter });
    expect(isAcceleratorBrowserFacade(accessor)).toBe(false);
    expect(getter).not.toHaveBeenCalled();
  });
});
