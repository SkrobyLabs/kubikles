import type { SecretEvent, SecretReadSource, WatcherError, WatcherStatus } from '../features/config/secrets/secretReadSourceContract';
import type { AcceleratorBrowserEvent, AcceleratorBrowserFacade, AcceleratorSecretDataEntry, AcceleratorSecretSummary } from './facade';

const SOURCE_KEY = 'accelerator-browser';
const HELM_SECRET_TYPE = 'helm.sh/release.v1';
type OwnedSpec = { namespace: string; excludeHelmReleases: boolean };
type PendingSpec = OwnedSpec & { candidates: SecretEvent[] };

const projectSummary = (value: any): AcceleratorSecretSummary | null => {
  const metadata = value?.metadata;
  if (!metadata || !stringField(metadata.name) || !stringField(metadata.namespace) ||
      !stringField(metadata.uid) || !stringField(metadata.creationTimestamp) ||
      typeof value?.type !== 'string' || !countField(value?.dataKeys)) return null;
  return {
    metadata: {
      name: metadata.name,
      namespace: metadata.namespace,
      uid: metadata.uid,
      creationTimestamp: metadata.creationTimestamp,
    },
    type: value.type,
    dataKeys: value.dataKeys,
  };
};

const projectDataEntry = (value: any): AcceleratorSecretDataEntry | null => {
  if (!value || !stringField(value.key) || typeof value.value !== 'string' ||
      typeof value.base64Value !== 'string' || typeof value.isBinary !== 'boolean' ||
      value.source !== 'data' || !['text', 'base64'].includes(value.encoding) ||
      (value.isBinary ? value.encoding !== 'base64' : value.encoding !== 'text')) return null;
  return {
    key: value.key,
    value: value.value,
    base64Value: value.base64Value,
    isBinary: value.isBinary,
    source: value.source,
    encoding: value.encoding,
  };
};

export type BrowserSecretReadSourceHandle = {
  source: SecretReadSource;
  onTerminal(callback: () => void): () => void;
  attach(): boolean;
  suspend(): void;
  dispose(): void;
};

const exactKeys = (value: unknown, expected: string[]) => {
  if (!value || typeof value !== 'object') return false;
  const keys = Reflect.ownKeys(value);
  return !keys.some(key => typeof key === 'symbol') && keys.filter((key): key is string => typeof key === 'string').sort().join('\0') === [...expected].sort().join('\0');
};
const stringField = (value: unknown): value is string => typeof value === 'string' && value.length > 0;
const countField = (value: unknown) => typeof value === 'number' && Number.isFinite(value) && Number.isInteger(value) && value >= 0;

export function createBrowserSecretReadSource(facade: AcceleratorBrowserFacade): BrowserSecretReadSourceHandle {
  const specs = new Map<string, OwnedSpec>();
  const pending = new Set<PendingSpec>();
  const listeners = {
    progress: new Set<(event: any) => void>(),
    resource: new Set<(event: SecretEvent) => void>(),
    status: new Set<(event: WatcherStatus) => void>(),
    error: new Set<(event: WatcherError) => void>(),
    connected: new Set<(event: any) => void>(),
    terminal: new Set<() => void>(),
  };
  let terminal = false;
  let disposed = false;
  let suspended = true;
  let detachFacade: (() => void) | null = null;

  const publish = (set: Set<(event: any) => void>, event: any) => {
    if (terminal || disposed || suspended) return;
    for (const callback of [...set]) callback(event);
  };
  const ownsNamespace = (spec: OwnedSpec, namespace: string) => spec.namespace === '' || spec.namespace === namespace;

  const receive = (event: AcceleratorBrowserEvent) => {
    if (disposed || suspended || !event || typeof event !== 'object') return;
    if (event.type === 'terminal') {
      if (!exactKeys(event, ['type'])) return;
      if (terminal) return;
      terminal = true;
      for (const callback of [...listeners.terminal]) callback();
      return;
    }
    if (terminal || !exactKeys(event, ['type', 'data'])) return;
    const data: any = event.data;
    if (event.type === 'resource-event') {
      if (!exactKeys(data, ['type', 'resourceType', 'namespace', 'watcherSpecId', 'resource']) || data.resourceType !== 'secrets' ||
          !['ADDED', 'MODIFIED', 'DELETED'].includes(data.type) || !stringField(data.namespace) || !stringField(data.watcherSpecId)) return;
      const summary = projectSummary(data.resource);
      if (!summary || data.namespace !== summary.metadata.namespace) return;
      const projected = { type: data.type, resourceType: 'secrets', namespace: data.namespace, watcherSpecId: data.watcherSpecId, resource: summary, sourceKey: SOURCE_KEY };
      const spec = specs.get(data.watcherSpecId);
      if (spec && ownsNamespace(spec, data.namespace) && !(spec.excludeHelmReleases && summary.type === HELM_SECRET_TYPE)) {
        publish(listeners.resource, projected);
        return;
      }
      for (const candidate of pending) {
        if (!ownsNamespace(candidate, data.namespace) || (candidate.excludeHelmReleases && summary.type === HELM_SECRET_TYPE)) continue;
        if (candidate.candidates.length === 64) candidate.candidates.shift();
        candidate.candidates.push(projected);
      }
      return;
    }
    if (event.type === 'watcher-status' && exactKeys(data, ['watcherSpecId', 'status']) && stringField(data.watcherSpecId) &&
        ['connected', 'reconnecting'].includes(data.status) && specs.has(data.watcherSpecId)) {
      publish(listeners.status, { watcherSpecId: data.watcherSpecId, status: data.status, sourceKey: SOURCE_KEY });
    } else if (event.type === 'watcher-error' && exactKeys(data, ['watcherSpecId', 'code', 'recoverable']) && stringField(data.watcherSpecId) &&
        ['watch_unavailable', 'resource_version_expired', 'malformed_watch_event'].includes(data.code) &&
        typeof data.recoverable === 'boolean' && specs.has(data.watcherSpecId)) {
      publish(listeners.error, { watcherSpecId: data.watcherSpecId, code: data.code, recoverable: data.recoverable, sourceKey: SOURCE_KEY });
    } else if (event.type === 'connected' && exactKeys(data, ['resumed']) && typeof data.resumed === 'boolean') {
      publish(listeners.connected, { resumed: data.resumed, sourceKey: SOURCE_KEY });
    } else if (event.type === 'list-progress' && exactKeys(data, ['resourceType', 'requestId', 'loaded', 'total']) &&
        data.resourceType === 'secrets' && stringField(data.requestId) && countField(data.loaded) && countField(data.total)) {
      publish(listeners.progress, { resourceType: 'secrets', requestId: data.requestId, loaded: data.loaded, total: data.total, sourceKey: SOURCE_KEY });
    }
  };

  const on = <T>(set: Set<(event: T) => void>, callback: (event: T) => void) => {
    if (disposed) return () => {};
    if (set === listeners.terminal && terminal) {
      callback(undefined as T);
      return () => {};
    }
    set.add(callback);
    return () => set.delete(callback);
  };
  const assertActive = () => {
    if (terminal || disposed || suspended) return Promise.reject(new Error('Secret Browser is unavailable'));
    return null;
  };

  const source: SecretReadSource = {
    sourceKey: SOURCE_KEY,
    list: async (requestId, namespace, excludeHelmReleases) => {
      const inactive = assertActive();
      if (inactive) return inactive;
      const result = await facade.ListSecretsMetadata(requestId, namespace, excludeHelmReleases);
      if (terminal || disposed) return [];
      if (!Array.isArray(result)) throw new Error('Unable to load Secrets');
      return result.map(projectSummary).filter((value): value is AcceleratorSecretSummary => value !== null)
        .filter(value => !(excludeHelmReleases && value.type === HELM_SECRET_TYPE));
    },
    cancelList: requestId => facade.CancelListRequest(requestId),
    subscribe: async (namespace, excludeHelmReleases) => {
      const inactive = assertActive();
      if (inactive) return inactive;
      const candidate: PendingSpec = { namespace, excludeHelmReleases, candidates: [] };
      pending.add(candidate);
      try {
        const result = await facade.SubscribeSecretWatcher(namespace, excludeHelmReleases);
        const watcherSpecId = stringField(result) ? result :
          exactKeys(result, ['watcherSpecId']) && stringField((result as { watcherSpecId?: unknown }).watcherSpecId) ? (result as { watcherSpecId: string }).watcherSpecId : '';
        if (terminal || disposed) {
          if (watcherSpecId) await facade.UnsubscribeSecretWatcher(watcherSpecId);
          return '';
        }
        if (watcherSpecId) {
          specs.set(watcherSpecId, candidate);
          for (const event of candidate.candidates) {
            if (event.watcherSpecId === watcherSpecId) publish(listeners.resource, event);
          }
        }
        return watcherSpecId ?? '';
      } finally {
        pending.delete(candidate);
      }
    },
    unsubscribe: async watcherSpecId => {
      specs.delete(watcherSpecId);
      return facade.UnsubscribeSecretWatcher(watcherSpecId);
    },
    getSecretData: async (namespace, name) => {
      const inactive = assertActive();
      if (inactive) return inactive;
      const result = await facade.GetSecretData(namespace, name);
      if (terminal || disposed) return [];
      if (!Array.isArray(result)) throw new Error('Unable to load Secret detail');
      return result.map(projectDataEntry).filter((value): value is AcceleratorSecretDataEntry => value !== null);
    },
    getSecretYaml: async (namespace, name) => {
      const inactive = assertActive();
      if (inactive) return inactive;
      const result = await facade.GetSecretYaml(namespace, name);
      if (terminal || disposed) return '';
      if (typeof result !== 'string') throw new Error('Unable to load Secret detail');
      return result;
    },
    onProgress: callback => on(listeners.progress, callback),
    onResource: callback => on(listeners.resource, callback),
    onStatus: callback => on(listeners.status, callback),
    onError: callback => on(listeners.error, callback),
    onConnected: callback => on(listeners.connected, callback),
  };

  return {
    source,
    onTerminal: callback => on(listeners.terminal, callback),
    attach() {
      if (disposed) return false;
      suspended = false;
      if (detachFacade) return !terminal;
      const detach = facade.events.subscribe(receive);
      if (disposed) {
        if (typeof detach === 'function') detach();
        return false;
      }
      detachFacade = typeof detach === 'function' ? detach : () => {};
      return !terminal;
    },
    suspend() {
      suspended = true;
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      suspended = true;
      detachFacade?.();
      detachFacade = null;
      specs.clear();
      pending.clear();
      Object.values(listeners).forEach(set => set.clear());
    },
  };
}
