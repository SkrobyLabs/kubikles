import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SecretReadSource } from './secretReadSource';
import { applySecretEvent, normalizeSecretNamespaces, SecretListOperationController, type SecretListState } from './secretListOperations';
import Logger from '~/utils/Logger';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: any) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

const flush = async () => {
  for (let index = 0; index < 10; index += 1) await Promise.resolve();
};
const last = <T,>(values: T[]) => values[values.length - 1];

class FakeSource implements SecretReadSource {
  sourceKey: string;
  autoSubscribe = true;
  subscribeCalls: Array<{ namespace: string; exclude: boolean; deferred: ReturnType<typeof deferred<string>> }> = [];
  listCalls: Array<{ id: string; namespace: string; exclude: boolean; deferred: ReturnType<typeof deferred<any[]>> }> = [];
  cancelCalls: string[] = [];
  unsubscribeCalls: string[] = [];
  unsubscribeDeferred: ReturnType<typeof deferred<any>> | null = null;
  listeners = {
    progress: new Set<(event: any) => void>(),
    resource: new Set<(event: any) => void>(),
    status: new Set<(event: any) => void>(),
    error: new Set<(event: any) => void>(),
    connected: new Set<(event: any) => void>(),
  };
  disposed = 0;

  constructor(sourceKey = 'fake-source') { this.sourceKey = sourceKey; }
  list(id: string, namespace: string, exclude: boolean) {
    const item = { id, namespace, exclude, deferred: deferred<any[]>() };
    this.listCalls.push(item);
    return item.deferred.promise;
  }
  cancelList(id: string) { this.cancelCalls.push(id); return Promise.resolve(); }
  subscribe(namespace: string, exclude: boolean) {
    const item = { namespace, exclude, deferred: deferred<string>() };
    this.subscribeCalls.push(item);
    if (this.autoSubscribe) item.deferred.resolve(`spec:${namespace || 'all'}:${exclude}`);
    return item.deferred.promise;
  }
  unsubscribe(id: string) {
    this.unsubscribeCalls.push(id);
    return this.unsubscribeDeferred?.promise ?? Promise.resolve();
  }
  getSecretData = vi.fn(async () => []);
  getSecretYaml = vi.fn(async () => '');
  private on(kind: keyof FakeSource['listeners'], callback: (event: any) => void) {
    this.listeners[kind].add(callback);
    return () => { if (this.listeners[kind].delete(callback)) this.disposed += 1; };
  }
  onProgress = (callback: (event: any) => void) => this.on('progress', callback);
  onResource = (callback: (event: any) => void) => this.on('resource', callback);
  onStatus = (callback: (event: any) => void) => this.on('status', callback);
  onError = (callback: (event: any) => void) => this.on('error', callback);
  onConnected = (callback: (event: any) => void) => this.on('connected', callback);
  emit(kind: keyof FakeSource['listeners'], event: any) {
    for (const callback of [...this.listeners[kind]]) callback({ ...event, sourceKey: event.sourceKey ?? this.sourceKey });
  }
}

const row = (uid: string, namespace = 'a', type = 'Opaque') => ({ type, metadata: { uid, namespace, name: uid } });
const productionDiagnostic = (event: 'subscription_failure' | 'list_failure', requestId: string, namespace: string) => {
  if (typeof window === 'undefined') return;
  Logger.error('Secret list operation failure', { event, requestId, namespace: namespace || 'all-namespaces' }, 'config');
};

let loggedEvents: any[];
let consoleError: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  loggedEvents = [];
  consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.stubGlobal('window', {
    dispatchEvent: (event: { detail: unknown }) => {
      loggedEvents.push(event.detail);
      return true;
    },
  });
  vi.stubGlobal('CustomEvent', class {
    detail: unknown;
    constructor(_type: string, init: { detail: unknown }) { this.detail = init.detail; }
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('SecretListOperationController', () => {
  it('owns one logical operation and unique namespace children', async () => {
    expect(normalizeSecretNamespaces([], ['a', 'b'])).toEqual([]);
    expect(normalizeSecretNamespaces('*', ['a', 'b'])).toEqual(['']);
    expect(normalizeSecretNamespaces('', ['a', 'b'])).toEqual(['']);
    expect(normalizeSecretNamespaces(['b', 'a', 'b'], ['a', 'b', 'c'])).toEqual(['a', 'b']);
    expect(normalizeSecretNamespaces(['b', 'a'], ['a', 'b'])).toEqual(['']);
    expect(applySecretEvent(new Map(), { type: 'UNKNOWN', resource: row('x') })).toEqual(new Map());

    const source = new FakeSource();
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state));
    await controller.replace(source, ['a', 'b'], true, true);
    await flush();
    expect(source.subscribeCalls.map(call => [call.namespace, call.exclude])).toEqual([['a', true], ['b', true]]);
    expect(source.listCalls).toHaveLength(2);
    const [a, b] = source.listCalls;
    expect(a.id).toMatch(/^secret-\d+-0-a$/);
    expect(b.id).toMatch(/^secret-\d+-1-b$/);
    expect(a.id).not.toBe(b.id);

    source.emit('progress', { resourceType: 'secrets', requestId: a.id, loaded: 2, total: 5 });
    source.emit('progress', { resourceType: 'secrets', requestId: b.id, loaded: 3, total: 7 });
    source.emit('progress', { resourceType: 'secrets', requestId: 'unknown', loaded: 100, total: 100 });
    expect(last(states)?.loadingProgress).toEqual({ loaded: 5, total: 12 });
    expect(last(states)?.secrets).toBe(states[states.length - 2]?.secrets);
    a.deferred.resolve([row('a')]);
    await flush();
    expect(last(states)?.loadingProgress).toEqual({ loaded: 3, total: 7 });

    controller.reconcile();
    await flush();
    expect(source.cancelCalls).toEqual([b.id]);
    expect(source.cancelCalls).not.toContain(a.id);
    expect(source.listCalls).toHaveLength(4);
    source.listCalls[2].deferred.resolve([row('a2', 'a')]);
    source.listCalls[3].deferred.resolve([row('b2', 'b')]);
    await flush();
    expect(last(states)?.secrets.map(secret => secret.metadata.uid)).toEqual(['a2', 'b2']);
    await controller.stop();
  });

  it('stale operation work cannot commit', async () => {
    const sourceA = new FakeSource('source-a');
    const sourceB = new FakeSource('source-b');
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(sourceA, state => states.push(state));
    await controller.replace(sourceA, ['a'], true, true);
    await flush();
    const oldList = sourceA.listCalls[0];
    sourceA.unsubscribeDeferred = deferred<any>();
    const replacement = controller.replace(sourceB, ['b'], false, true);
    sourceA.emit('progress', { resourceType: 'secrets', requestId: oldList.id, loaded: 9, total: 9 });
    oldList.deferred.resolve([row('stale')]);
    await flush();
    expect(sourceA.disposed).toBe(5);
    expect(sourceA.cancelCalls).toEqual([oldList.id]);
    expect(sourceB.subscribeCalls).toHaveLength(0);
    expect(last(states)?.secrets).toEqual([]);
    sourceA.unsubscribeDeferred.resolve(undefined);
    await replacement;
    await flush();
    expect(sourceB.subscribeCalls.map(call => call.namespace)).toEqual(['b']);
    sourceB.listCalls[0].deferred.resolve([row('fresh', 'b')]);
    await flush();
    expect(last(states)?.secrets.map(secret => secret.metadata.uid)).toEqual(['fresh']);
    sourceA.emit('progress', { resourceType: 'secrets', requestId: oldList.id, loaded: 10, total: 10 });
    expect(last(states)?.loadingProgress).toBeNull();
    await controller.stop();
  });

  it('synchronously fences visible state while an old unsubscribe is blocked', async () => {
    const sourceA = new FakeSource('source-a');
    const sourceB = new FakeSource('source-b');
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(sourceA, state => states.push(state));
    await controller.replace(sourceA, ['a'], false, true);
    await flush();
    sourceA.listCalls[0].deferred.resolve([row('old')]);
    await flush();
    sourceA.unsubscribeDeferred = deferred<any>();
    const replacement = controller.replace(sourceB, ['b'], true, true);
    expect(last(states)).toMatchObject({ secrets: [], loading: false, error: null, loadingProgress: null });
    expect(sourceB.subscribeCalls).toHaveLength(0);
    await flush();
    expect(sourceB.subscribeCalls).toHaveLength(0);
    sourceA.unsubscribeDeferred.resolve(undefined);
    await replacement;
    await flush();
    expect(sourceB.subscribeCalls.map(call => call.namespace)).toEqual(['b']);
    await controller.stop();
  });

  it('retains the emitted rows identity for initial and reconciliation progress', async () => {
    const source = new FakeSource();
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state));
    await controller.replace(source, ['a'], false, true);
    await flush();
    const initial = source.listCalls[0];
    const beforeFirstProgress = last(states)!.secrets;
    source.emit('progress', { resourceType: 'secrets', requestId: initial.id, loaded: 1, total: 2 });
    expect(last(states)!.secrets).toBe(beforeFirstProgress);
    initial.deferred.resolve([row('committed')]);
    await flush();
    const committed = last(states)!.secrets;
    controller.reconcile();
    await flush();
    const reconciliation = source.listCalls[1];
    source.emit('progress', { resourceType: 'secrets', requestId: reconciliation.id, loaded: 1, total: 2 });
    expect(last(states)!.secrets).toBe(committed);
    reconciliation.deferred.resolve([row('reconciled')]);
    await flush();
    await controller.stop();
  });

  it('owns exact watcher specs and cleans late subscriptions', async () => {
    const source = new FakeSource();
    source.autoSubscribe = false;
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state));
    await controller.replace(source, ['a', 'b'], true, true);
    expect(source.subscribeCalls).toHaveLength(2);
    source.subscribeCalls[1].deferred.reject(new Error('subscription bound'));
    await flush();
    expect(source.listCalls.map(call => call.namespace)).toEqual(['b']);

    const replacement = controller.replace(source, ['a'], false, true);
    await flush();
    expect(source.subscribeCalls).toHaveLength(2);
    source.subscribeCalls[0].deferred.resolve('stable-spec');
    await replacement;
    await flush();
    expect(source.unsubscribeCalls).toEqual(['stable-spec']);
    expect(source.subscribeCalls).toHaveLength(3);
    source.subscribeCalls[2].deferred.resolve('stable-spec');
    await flush();
    expect(source.listCalls.map(call => call.namespace)).toEqual(['b', 'a']);

    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'wrong', namespace: 'a', resource: row('wrong') });
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'stable-spec', namespace: 'b', resource: row('wrong-ns', 'b') });
    expect(last(states)?.secrets).toEqual([]);
    source.listCalls[1].deferred.resolve([row('current')]);
    await flush();
    expect(last(states)?.secrets.map(secret => secret.metadata.uid)).toEqual(['current']);
    await controller.stop();
    expect(source.unsubscribeCalls).toEqual(['stable-spec', 'stable-spec']);
    expect(source.disposed).toBe(10);
  });

  it('classifies the original subscription failure while still starting its list', async () => {
    const source = new FakeSource();
    source.autoSubscribe = false;
    const diagnostics: unknown[] = [];
    const controller = new SecretListOperationController(source, () => {}, error => { diagnostics.push(error); return false; });
    await controller.replace(source, ['named'], false, true);
    const failure = new Error('Accelerator Secret watch subscription limit reached');
    source.subscribeCalls[0].deferred.reject(failure);
    await flush();
    expect(source.listCalls).toHaveLength(1);
    expect(source.subscribeCalls).toHaveLength(1);
    expect(diagnostics).toEqual([failure]);
    source.listCalls[0].deferred.resolve([]);
    await flush();
    await controller.stop();
  });

  it('uses production logging with sanitized subscription diagnostics only for current non-cancellations', async () => {
    const source = new FakeSource();
    source.autoSubscribe = false;
    const classified: unknown[] = [];
    const controller = new SecretListOperationController(source, () => {}, error => { classified.push(error); return true; }, productionDiagnostic);

    await controller.replace(source, [''], false, true);
    const hostile = Object.assign(new Error('Unauthorized HOSTILE_SUBSCRIPTION_BODY'), {
      resource: row('HOSTILE_SECRET_UID'),
      credential: 'HOSTILE_CREDENTIAL',
    });
    source.subscribeCalls[0].deferred.reject(hostile);
    await flush();

    expect(classified).toEqual([hostile]);
    expect(source.listCalls).toHaveLength(1);
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(loggedEvents).toEqual([{
      category: 'config',
      message: 'Secret list operation failure',
      details: {
        level: 'ERROR',
        data: {
          event: 'subscription_failure',
          requestId: source.listCalls[0].id,
          namespace: 'all-namespaces',
        },
      },
    }]);
    const logged = JSON.stringify({ console: consoleError.mock.calls, events: loggedEvents });
    expect(logged).not.toMatch(/HOSTILE_SUBSCRIPTION_BODY|HOSTILE_SECRET_UID|HOSTILE_CREDENTIAL|Unauthorized/);

    source.listCalls[0].deferred.resolve([]);
    await flush();
    await controller.replace(source, ['cancelled'], false, true);
    source.subscribeCalls[1].deferred.reject(new Error('cancelled HOSTILE_CANCELLED_BODY'));
    await flush();
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(classified).toEqual([hostile]);

    await controller.replace(source, ['stale'], false, true);
    const stale = source.subscribeCalls[2];
    const replacement = controller.replace(source, ['current'], false, true);
    stale.deferred.reject(new Error('HOSTILE_STALE_BODY'));
    await replacement;
    await flush();
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(classified).toEqual([hostile]);
    source.subscribeCalls[3].deferred.resolve('current-spec');
    await flush();
    source.listCalls[source.listCalls.length - 1].deferred.resolve([]);
    await flush();
    await controller.stop();
  });

  it('uses production logging for a named list failure without exposing its error or Secret', async () => {
    const source = new FakeSource();
    const states: SecretListState[] = [];
    const classified: unknown[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state), error => { classified.push(error); return true; }, productionDiagnostic);

    await controller.replace(source, ['named'], false, true);
    await flush();
    const request = source.listCalls[0];
    const hostile = Object.assign(new Error('x509 HOSTILE_LIST_BODY'), {
      secret: row('HOSTILE_LIST_SECRET_UID'),
      keyName: 'HOSTILE_KEY_NAME',
    });
    request.deferred.reject(hostile);
    await flush();

    expect(classified).toEqual([hostile]);
    expect(last(states)).toMatchObject({ secrets: [], error: null, loading: false });
    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(loggedEvents).toEqual([{
      category: 'config',
      message: 'Secret list operation failure',
      details: {
        level: 'ERROR',
        data: {
          event: 'list_failure',
          requestId: request.id,
          namespace: 'named',
        },
      },
    }]);
    const logged = JSON.stringify({ console: consoleError.mock.calls, events: loggedEvents });
    expect(logged).not.toMatch(/HOSTILE_LIST_BODY|HOSTILE_LIST_SECRET_UID|HOSTILE_KEY_NAME|x509/);
    await controller.stop();
  });

  it('buffers watch events across list and reconciles gaps', async () => {
    const source = new FakeSource();
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state));
    await controller.replace(source, ['a'], true, true);
    await flush();
    const initial = source.listCalls[0];
    const spec = 'spec:a:true';
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: spec, namespace: 'a', resource: row('added') });
    source.emit('resource', { type: 'MODIFIED', resourceType: 'secrets', watcherSpecId: spec, namespace: 'a', resource: { ...row('base'), value: 'new' } });
    source.emit('resource', { type: 'DELETED', resourceType: 'secrets', watcherSpecId: spec, namespace: 'a', resource: row('deleted') });
    source.emit('resource', { type: 'ADDED', resourceType: 'configmaps', watcherSpecId: spec, namespace: 'a', resource: row('wrong-type') });
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: spec, namespace: 'a', resource: row('helm', 'a', 'helm.sh/release.v1') });
    initial.deferred.resolve([{ ...row('base'), value: 'old' }, row('deleted')]);
    await flush();
    expect(last(states)?.secrets.map(secret => [secret.metadata.uid, secret.value])).toEqual([['base', 'new'], ['added', undefined]]);

    source.emit('connected', { resumed: false });
    source.emit('status', { watcherSpecId: spec, status: 'running' });
    source.emit('error', { watcherSpecId: spec, code: 'unrelated' });
    await flush();
    expect(source.listCalls).toHaveLength(1);
    source.emit('connected', { resumed: true });
    source.emit('status', { watcherSpecId: spec, status: 'reconnecting' });
    source.emit('error', { watcherSpecId: spec, code: 'resource_version_expired' });
    await flush();
    expect(source.listCalls).toHaveLength(2);
    expect(source.subscribeCalls).toHaveLength(1);
    const reconciliation = source.listCalls[1];
    source.emit('status', { watcherSpecId: spec, status: 'reconnecting' });
    await flush();
    expect(source.cancelCalls).toContain(reconciliation.id);
    expect(source.listCalls).toHaveLength(3);
    source.listCalls[2].deferred.resolve([row('reconciled')]);
    reconciliation.deferred.resolve([row('obsolete')]);
    await flush();
    expect(last(states)?.secrets.map(secret => secret.metadata.uid)).toEqual(['reconciled']);
    expect(source.subscribeCalls).toHaveLength(1);
    await controller.stop();
  });

  it('installs raw listeners before subscribing and retains only exact pending events across listing', async () => {
    const source = new FakeSource();
    source.autoSubscribe = false;
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state));
    await controller.replace(source, ['a'], true, true);
    expect(source.listeners.resource.size).toBe(1);
    expect(source.subscribeCalls).toHaveLength(1);

    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'owned-spec', namespace: 'a', resource: { ...row('event'), dataKeys: 1, data: { private: 'value' } } });
    source.emit('resource', { type: 'MODIFIED', resourceType: 'secrets', watcherSpecId: 'owned-spec', namespace: 'a', resource: { ...row('event'), dataKeys: 2, stringData: { private: 'value' } } });
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'wrong-spec', namespace: 'a', resource: row('wrong-id') });
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'owned-spec', namespace: 'b', resource: row('wrong-namespace', 'b') });
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'owned-spec', namespace: 'a', resource: row('helm', 'a', 'helm.sh/release.v1') });
    source.emit('resource', { type: 'ADDED', resourceType: 'configmaps', watcherSpecId: 'owned-spec', namespace: 'a', resource: row('wrong-type') });
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'owned-spec', namespace: 'a', resource: row('wrong-source'), sourceKey: 'obsolete-source' });
    expect(last(states)?.secrets).toEqual([]);
    expect(source.listCalls).toHaveLength(0);

    source.subscribeCalls[0].deferred.resolve('owned-spec');
    await flush();
    expect(source.listCalls).toHaveLength(1);
    source.listCalls[0].deferred.resolve([row('listed')]);
    await flush();
    expect(last(states)!.secrets.map(value => [value.metadata.uid, value.dataKeys])).toEqual([['listed', undefined], ['event', 2]]);
    expect(JSON.stringify(last(states)?.secrets)).not.toMatch(/"data"|"stringData"|private|value/);
    await controller.stop();
  });

  it('bounds pending candidates deterministically and clears failed, empty, and obsolete generations', async () => {
    const source = new FakeSource();
    source.autoSubscribe = false;
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state));
    await controller.replace(source, ['a'], false, true);
    for (let index = 0; index < 70; index += 1) {
      source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'bounded-spec', namespace: 'a', resource: row(`bounded-${index}`) });
    }
    source.subscribeCalls[0].deferred.resolve('bounded-spec');
    await flush();
    source.listCalls[0].deferred.resolve([]);
    await flush();
    expect(last(states)?.secrets.map(value => value.metadata.uid)).toEqual(Array.from({ length: 64 }, (_, index) => `bounded-${index + 6}`));

    await controller.replace(source, ['failed', 'empty'], false, true);
    const failedCall = source.subscribeCalls[1];
    const emptyCall = source.subscribeCalls[2];
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'failed-spec', namespace: 'failed', resource: row('failed-event', 'failed') });
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'empty-spec', namespace: 'empty', resource: row('empty-event', 'empty') });
    failedCall.deferred.reject(new Error('private subscription failure'));
    emptyCall.deferred.resolve('');
    await flush();
    expect(source.listCalls.slice(1)).toHaveLength(2);
    source.listCalls[1].deferred.resolve([]);
    source.listCalls[2].deferred.resolve([]);
    await flush();
    expect(last(states)?.secrets).toEqual([]);

    const obsolete = new FakeSource('obsolete-source');
    obsolete.autoSubscribe = false;
    await controller.replace(obsolete, ['old'], false, true);
    obsolete.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'old-spec', namespace: 'old', resource: row('old-event', 'old') });
    const replacement = controller.replace(source, ['new'], false, true);
    obsolete.subscribeCalls[0].deferred.resolve('old-spec');
    await replacement;
    expect(obsolete.unsubscribeCalls).toEqual(['old-spec']);
    source.emit('resource', { type: 'ADDED', resourceType: 'secrets', watcherSpecId: 'old-spec', namespace: 'old', resource: row('late-old', 'old'), sourceKey: 'obsolete-source' });
    const newCall = source.subscribeCalls[3];
    newCall.deferred.resolve('new-spec');
    await flush();
    source.listCalls[3].deferred.resolve([]);
    await flush();
    expect(last(states)?.secrets).toEqual([]);
    await controller.stop();
  });

  it('preserves named partial success and all-namespace error handling', async () => {
    const source = new FakeSource();
    const errors: unknown[] = [];
    const states: SecretListState[] = [];
    const controller = new SecretListOperationController(source, state => states.push(state), error => { errors.push(error); return true; });
    await controller.replace(source, ['a', 'b'], false, true);
    await flush();
    source.listCalls[0].deferred.reject(new Error('a failed'));
    source.listCalls[1].deferred.resolve([row('b', 'b')]);
    await flush();
    expect(errors).toHaveLength(1);
    expect(last(states)?.error).toBeNull();
    expect(last(states)?.secrets.map(secret => secret.metadata.uid)).toEqual(['b']);
    await controller.replace(source, ['named'], false, true);
    await flush();
    last(source.listCalls)!.deferred.reject(new Error('named failed'));
    await flush();
    expect(last(states)).toMatchObject({ secrets: [], error: null });
    expect((errors[errors.length - 1] as Error).message).toBe('named failed');
    await controller.replace(source, [''], false, true);
    await flush();
    last(source.listCalls)!.deferred.reject(new Error('all failed'));
    await flush();
    expect((last(states)?.error as Error).message).toBe('Secret list failed for namespace all namespaces');
    source.unsubscribeDeferred = deferred<any>();
    const inactive = controller.replace(source, [], false, false);
    expect(last(states)).toMatchObject({ secrets: [], loading: false, error: null, loadingProgress: null });
    source.unsubscribeDeferred.resolve(undefined);
    await inactive;
  });
});
