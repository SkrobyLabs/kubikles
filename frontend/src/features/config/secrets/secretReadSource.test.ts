import { beforeEach, describe, expect, it, vi } from 'vitest';

const bridge = vi.hoisted(() => ({
  CancelIntegratedSecretListRequest: vi.fn(),
  CancelListRequest: vi.fn(),
  GetIntegratedSecretData: vi.fn(),
  GetIntegratedSecretYaml: vi.fn(),
  GetSecretData: vi.fn(),
  GetSecretYaml: vi.fn(),
  ListAcceleratorSecretsMetadata: vi.fn(),
  ListHelmReleaseMetadata: vi.fn(),
  ListIntegratedHelmReleaseMetadata: vi.fn(),
  ListIntegratedSecretsMetadata: vi.fn(),
  ListSecretsMetadata: vi.fn(),
  ReleaseIntegratedSecretReads: vi.fn(),
  RetainIntegratedSecretReads: vi.fn(),
  SubscribeIntegratedSecretWatcher: vi.fn(),
  SubscribeResourceWatcher: vi.fn(),
  SubscribeSecretWatcher: vi.fn(),
  UnsubscribeSecretWatcher: vi.fn(),
  UnsubscribeIntegratedSecretWatcher: vi.fn(),
  UnsubscribeWatcher: vi.fn(),
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
    emit(name: string, value: any) {
      for (const callback of [...(listeners.get(name) ?? [])]) callback(value);
    },
  };
});

vi.mock('wailsjs/go/main/App', () => bridge);
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: runtime.EventsOn }));

import { createIntegratedAcceleratorSecretReadSource, directSecretReadSource } from './secretReadSource';
import { SecretListOperationController, type SecretListState } from './secretListOperations';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: any) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

const flush = async () => {
  for (let index = 0; index < 10; index += 1) await Promise.resolve();
};

describe('Secret read sources', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    runtime.listeners.clear();
    bridge.CancelListRequest.mockResolvedValue(undefined);
    bridge.UnsubscribeWatcher.mockResolvedValue(undefined);
    bridge.UnsubscribeSecretWatcher.mockResolvedValue(undefined);
    bridge.CancelIntegratedSecretListRequest.mockResolvedValue(true);
    bridge.UnsubscribeIntegratedSecretWatcher.mockResolvedValue(undefined);
  });

  it('direct source preserves ordinary call shape', async () => {
    const normal = { type: 'Opaque', data: { key: 'value' }, metadata: { name: 'one', uid: 'one', namespace: 'a', creationTimestamp: '2024-01-01T00:00:00Z' } };
    const helm = { type: 'helm.sh/release.v1', metadata: { uid: 'helm', namespace: 'a' } };
    bridge.ListSecretsMetadata.mockResolvedValue([normal, helm]);
    bridge.ListHelmReleaseMetadata.mockResolvedValue([{ name: 'release' }]);
    bridge.SubscribeResourceWatcher.mockResolvedValue('direct:a');
    bridge.GetSecretData.mockResolvedValue([{ key: 'token' }]);
    bridge.GetSecretYaml.mockResolvedValue('yaml');

    await expect(directSecretReadSource.list('request-a', 'a', true)).resolves.toEqual([normal]);
    expect(bridge.ListSecretsMetadata).toHaveBeenCalledWith('request-a', 'a');
    expect(bridge.ListSecretsMetadata.mock.calls[0]).toHaveLength(2);
    await expect(directSecretReadSource.listHelmReleaseMetadata('helm-request', 'a')).resolves.toEqual([{ name: 'release' }]);
    expect(bridge.ListHelmReleaseMetadata).toHaveBeenCalledWith('helm-request', 'a');
    await expect(directSecretReadSource.subscribe('a', true)).resolves.toBe('direct:a');
    expect(bridge.SubscribeResourceWatcher).toHaveBeenCalledWith('secrets', 'a');

    const events: any[] = [];
    const dispose = directSecretReadSource.onResource(event => events.push(event));
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'b', resource: { ...normal, metadata: { uid: 'wrong', namespace: 'b' } } });
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', resource: helm });
    runtime.emit('resource-events-batch', [
      { type: 'ADDED', resourceType: 'pods', namespace: 'a', resource: normal },
      { type: 'ADDED', resourceType: 'secrets', namespace: 'a', resource: normal },
    ]);
    expect(events).toEqual([{ type: 'ADDED', resourceType: 'secrets', namespace: 'a', resource: { metadata: normal.metadata, type: 'Opaque', dataKeys: 1 }, watcherSpecId: 'direct:a', sourceKey: 'direct-secrets' }]);

    await expect(directSecretReadSource.getSecretData('a', 'one')).resolves.toEqual([{ key: 'token' }]);
    await expect(directSecretReadSource.getSecretYaml('a', 'one')).resolves.toBe('yaml');
    await directSecretReadSource.cancelList('request-a');
    expect(bridge.CancelListRequest).toHaveBeenCalledWith('request-a');
    dispose();
    expect(runtime.listeners.get('resource-event')?.size ?? 0).toBe(0);
    expect(runtime.listeners.get('resource-events-batch')?.size ?? 0).toBe(0);
    await directSecretReadSource.unsubscribe('direct:a');
    expect(bridge.UnsubscribeWatcher).toHaveBeenCalledWith('direct:a');
  });

  it('projects hostile direct single and batch Secret events to the closed summary shape', async () => {
    bridge.SubscribeResourceWatcher.mockResolvedValue('direct:a');
    await directSecretReadSource.subscribe('a', false);
    const events: any[] = [];
    const dispose = directSecretReadSource.onResource(event => events.push(event));
    const hostile = {
      type: 'Opaque', data: { password: 'c2VjcmV0' }, stringData: { token: 'sensitive-token' },
      metadata: { name: 'one', namespace: 'a', uid: 'uid-one', creationTimestamp: '2024-01-01T00:00:00Z', labels: { marker: 'sensitive-label' }, annotations: { marker: 'sensitive-annotation' } },
    };
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', resource: hostile });
    runtime.emit('resource-events-batch', [{ type: 'MODIFIED', resourceType: 'secrets', namespace: 'a', resource: { ...hostile, metadata: { ...hostile.metadata, name: 'two', uid: 'uid-two' } } }]);
    expect(events).toEqual([
      { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'direct:a', sourceKey: 'direct-secrets', resource: { metadata: { name: 'one', namespace: 'a', uid: 'uid-one', creationTimestamp: '2024-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 1 } },
      { type: 'MODIFIED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'direct:a', sourceKey: 'direct-secrets', resource: { metadata: { name: 'two', namespace: 'a', uid: 'uid-two', creationTimestamp: '2024-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 1 } },
    ]);
    expect(JSON.stringify(events)).not.toMatch(/password|stringData|labels|annotations|c2VjcmV0|sensitive-/);
    dispose();
    await directSecretReadSource.unsubscribe('direct:a');
  });

  it('normalizes null and non-array direct list successes before Helm filtering', async () => {
    bridge.ListSecretsMetadata.mockResolvedValueOnce(null).mockResolvedValueOnce({ not: 'rows' });
    await expect(directSecretReadSource.list('null', 'a', false)).resolves.toEqual([]);
    await expect(directSecretReadSource.list('object', 'a', true)).resolves.toEqual([]);
  });

  it('integrated source calls only closed token-scoped bridges and projects targeted events', async () => {
    const token = `s.${'A'.repeat(22)}.0000000000000001`;
    const source = createIntegratedAcceleratorSecretReadSource(token);
    bridge.ListIntegratedSecretsMetadata.mockResolvedValue([{ metadata: { uid: 'one' } }]);
    bridge.ListIntegratedHelmReleaseMetadata.mockResolvedValue([{ name: 'release' }]);
    bridge.GetIntegratedSecretData.mockResolvedValue([{ key: 'one' }]);
    bridge.GetIntegratedSecretYaml.mockResolvedValue('yaml');
    bridge.SubscribeIntegratedSecretWatcher.mockResolvedValue('spec-a');

    await source.list('request', 'a', true);
    await source.listHelmReleaseMetadata('helm-request', 'a');
    await source.getSecretData('a', 'one');
    await source.getSecretYaml('a', 'one');
    await source.cancelList('request');
    await expect(source.subscribe('a', true)).resolves.toBe('spec-a');
    expect(bridge.ListIntegratedSecretsMetadata).toHaveBeenCalledWith(token, 'request', 'a', true);
    expect(bridge.ListIntegratedHelmReleaseMetadata).toHaveBeenCalledWith(token, 'helm-request', 'a');
    expect(bridge.GetIntegratedSecretData).toHaveBeenCalledWith(token, 'a', 'one');
    expect(bridge.GetIntegratedSecretYaml).toHaveBeenCalledWith(token, 'a', 'one');
    expect(bridge.CancelIntegratedSecretListRequest).toHaveBeenCalledWith(token, 'request');
    expect(bridge.SubscribeIntegratedSecretWatcher).toHaveBeenCalledWith(token, 'a', true);

    const resources: any[] = [];
    const statuses: any[] = [];
    const errors: any[] = [];
    const disposeResource = source.onResource(event => resources.push(event));
    const disposeStatus = source.onStatus(event => statuses.push(event));
    const disposeError = source.onError(event => errors.push(event));
    const resource = { metadata: { name: 'one', namespace: 'a', uid: 'uid', creationTimestamp: 'now', labels: { private: 'marker' } }, type: 'Opaque', dataKeys: 1, data: { value: 'HOSTILE' } };
    runtime.emit('accelerator:secret-resource', { sourceToken: `s.${'B'.repeat(22)}.0000000000000002`, type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'spec-a', resource });
    runtime.emit('accelerator:secret-resource', { sourceToken: token, type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'spec-a', resource, raw: 'HOSTILE_RAW' });
    runtime.emit('accelerator:secret-watcher-status', { sourceToken: token, watcherSpecId: 'spec-a', status: 'connected', raw: 'HOSTILE_RAW' });
    runtime.emit('accelerator:secret-watcher-error', { sourceToken: token, watcherSpecId: 'spec-a', code: 'watch_unavailable', recoverable: true, raw: 'HOSTILE_RAW' });
    expect(resources).toEqual([{ type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'spec-a', sourceKey: token, resource: { metadata: { name: 'one', namespace: 'a', uid: 'uid', creationTimestamp: 'now' }, type: 'Opaque', dataKeys: 1 } }]);
    expect(statuses).toEqual([{ watcherSpecId: 'spec-a', status: 'connected', sourceKey: token }]);
    expect(errors).toEqual([{ watcherSpecId: 'spec-a', code: 'watch_unavailable', recoverable: true, sourceKey: token }]);
    expect(JSON.stringify([resources, statuses, errors])).not.toMatch(/HOSTILE|labels|"data":/);

    await source.unsubscribe('spec-a');
    expect(bridge.UnsubscribeIntegratedSecretWatcher).toHaveBeenCalledWith(token, 'spec-a');
    disposeResource(); disposeStatus(); disposeError();
    expect(() => createIntegratedAcceleratorSecretReadSource('session-identity')).toThrow(/unavailable/);
  });

  /* Browser-only standalone-source cases removed with the Accelerator Browser surface.
  it('retains only bounded projected candidates until the exact subscription ID resolves', async () => {
    const subscription = deferred<any>();
    bridge.SubscribeSecretWatcher.mockReturnValue(subscription.promise);
    const source = createAcceleratorSecretReadSource('accelerator-race');
    const events: any[] = [];
    const dispose = source.onResource(event => events.push(event));

    const pending = source.subscribe('a', true);
    expect(runtime.listeners.get('resource-event')?.size).toBe(1);
    expect(runtime.listeners.get('resource-events-batch')?.size).toBe(1);
    expect(bridge.SubscribeSecretWatcher).toHaveBeenCalledWith('a', true);

    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'wrong-id', resource: { type: 'Opaque', metadata: { uid: 'wrong-id', namespace: 'a' } } });
    for (let index = 0; index < 70; index += 1) {
      runtime.emit('resource-event', {
        type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'spec-race',
        resource: { type: 'Opaque', data: { password: 'c2VjcmV0' }, stringData: { token: 'raw-token' }, metadata: { name: `event-${index}`, namespace: 'a', uid: `event-${index}`, labels: { private: 'value' } } },
      });
    }
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'b', watcherSpecId: 'spec-race', resource: { type: 'Opaque', metadata: { uid: 'wrong-namespace', namespace: 'b' } } });
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'configmaps', namespace: 'a', watcherSpecId: 'spec-race', resource: { metadata: { uid: 'wrong-type', namespace: 'a' } } });
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'spec-race', resource: { type: 'helm.sh/release.v1', metadata: { uid: 'helm', namespace: 'a' } } });
    expect(events).toEqual([]);

    subscription.resolve({ watcherSpecId: 'spec-race' });
    await expect(pending).resolves.toBe('spec-race');
    expect(events).toHaveLength(64);
    expect(events.map(event => event.resource.metadata.uid)).toEqual(Array.from({ length: 64 }, (_, index) => `event-${index + 6}`));
    expect(JSON.stringify(events)).not.toMatch(/password|stringData|c2VjcmV0|raw-token|labels|private/);
    dispose();
  });

  it('detaches disposed pending consumers while preserving other live listeners', async () => {
    const sharedSubscription = deferred<any>();
    const obsoleteSubscription = deferred<any>();
    const currentSubscription = deferred<any>();
    bridge.SubscribeSecretWatcher
      .mockReturnValueOnce(sharedSubscription.promise)
      .mockReturnValueOnce(obsoleteSubscription.promise)
      .mockReturnValueOnce(currentSubscription.promise);
    const source = createAcceleratorSecretReadSource('accelerator-remount-race');
    const firstEvents: any[] = [];
    const secondEvents: any[] = [];
    const firstConsumer = (event: any) => firstEvents.push(event);
    const disposeFirst = source.onResource(firstConsumer);
    const disposeSecond = source.onResource(event => secondEvents.push(event));

    const sharedPending = source.subscribe('shared', false);
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'shared', watcherSpecId: 'shared-spec', resource: { type: 'Opaque', metadata: { uid: 'shared-event', namespace: 'shared' } } });
    disposeFirst();
    sharedSubscription.resolve({ watcherSpecId: 'shared-spec' });
    await expect(sharedPending).resolves.toBe('shared-spec');
    expect(firstEvents).toEqual([]);
    expect(secondEvents.map(event => event.resource.metadata.uid)).toEqual(['shared-event']);
    disposeSecond();

    const disposeObsolete = source.onResource(firstConsumer);
    const obsoletePending = source.subscribe('a', false);
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'obsolete-spec', resource: { type: 'Opaque', metadata: { uid: 'orphaned-event', namespace: 'a' } } });
    disposeObsolete();
    expect(runtime.listeners.get('resource-event')?.size ?? 0).toBe(0);
    expect(runtime.listeners.get('resource-events-batch')?.size ?? 0).toBe(0);

    const disposeCurrent = source.onResource(firstConsumer);
    const currentPending = source.subscribe('a', false);
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'obsolete-spec', resource: { type: 'Opaque', metadata: { uid: 'late-obsolete-event', namespace: 'a' } } });
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'current-spec', resource: { type: 'Opaque', metadata: { uid: 'current-event', namespace: 'a' } } });

    obsoleteSubscription.resolve({ watcherSpecId: 'obsolete-spec' });
    await expect(obsoletePending).resolves.toBe('obsolete-spec');
    expect(firstEvents).toEqual([]);
    await source.unsubscribe('obsolete-spec');
    expect(bridge.UnsubscribeSecretWatcher).toHaveBeenCalledWith('obsolete-spec');

    currentSubscription.resolve({ watcherSpecId: 'current-spec' });
    await expect(currentPending).resolves.toBe('current-spec');
    expect(firstEvents.map(event => event.resource.metadata.uid)).toEqual(['current-event']);
    disposeCurrent();
  });

  it('hands a pre-response exact-ID event to controller ownership before the later list commits', async () => {
    const subscription = deferred<any>();
    const list = deferred<any[]>();
    bridge.SubscribeSecretWatcher.mockReturnValue(subscription.promise);
    bridge.ListAcceleratorSecretsMetadata.mockReturnValue(list.promise);
    const source = createAcceleratorSecretReadSource('accelerator-integrated-race');
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state));

    await controller.replace(source, ['a'], false, true);
    expect(runtime.listeners.get('resource-event')?.size).toBe(1);
    expect(bridge.SubscribeSecretWatcher).toHaveBeenCalledWith('a', false);
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'integrated-spec', resource: { type: 'Opaque', dataKeys: 1, metadata: { uid: 'event', name: 'event', namespace: 'a' } } });
    runtime.emit('resource-event', { type: 'MODIFIED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'integrated-spec', resource: { type: 'Opaque', dataKeys: 2, metadata: { uid: 'event', name: 'event', namespace: 'a' } } });
    expect(states[states.length - 1].secrets).toEqual([]);
    expect(bridge.ListAcceleratorSecretsMetadata).not.toHaveBeenCalled();

    subscription.resolve({ watcherSpecId: 'integrated-spec' });
    await flush();
    expect(bridge.ListAcceleratorSecretsMetadata).toHaveBeenCalledWith(expect.stringMatching(/^secret-\d+-0-a$/), 'a', false);
    expect(states[states.length - 1].secrets).toEqual([]);
    list.resolve([{ type: 'Opaque', dataKeys: 1, metadata: { uid: 'listed', name: 'listed', namespace: 'a' } }]);
    await flush();
    expect(states[states.length - 1].secrets.map(secret => [secret.metadata.uid, secret.dataKeys])).toEqual([['listed', 1], ['event', 2]]);
    await controller.stop();
  });

  it('clears pending candidates on failed and empty accelerator subscriptions', async () => {
    const failed = deferred<any>();
    const empty = deferred<any>();
    bridge.SubscribeSecretWatcher.mockReturnValueOnce(failed.promise).mockReturnValueOnce(empty.promise);
    const source = createAcceleratorSecretReadSource('accelerator-failure');
    const events: any[] = [];
    const dispose = source.onResource(event => events.push(event));

    const failedPending = source.subscribe('a', false);
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'a', watcherSpecId: 'failed-id', resource: { type: 'Opaque', metadata: { uid: 'failed', namespace: 'a' } } });
    failed.reject(new Error('private backend failure'));
    await expect(failedPending).rejects.toThrow('private backend failure');

    const emptyPending = source.subscribe('b', false);
    runtime.emit('resource-event', { type: 'ADDED', resourceType: 'secrets', namespace: 'b', watcherSpecId: 'empty-id', resource: { type: 'Opaque', metadata: { uid: 'empty', namespace: 'b' } } });
    empty.resolve({ watcherSpecId: '' });
    await expect(emptyPending).resolves.toBe('');
    expect(events).toEqual([]);
    dispose();
  });
  */
});
