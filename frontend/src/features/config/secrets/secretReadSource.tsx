import React, { createContext, useContext } from 'react';
import {
  CancelListRequest,
  GetSecretData,
  GetSecretYaml,
  ListAcceleratorSecretsMetadata,
  ListSecretsMetadata,
  SubscribeResourceWatcher,
  SubscribeSecretWatcher,
  UnsubscribeSecretWatcher,
  UnsubscribeWatcher,
} from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import type { SecretEvent, SecretReadSource, WatcherError, WatcherStatus } from './secretReadSourceContract';
export type { SecretEvent, SecretListState, SecretReadSource, WatcherError, WatcherStatus } from './secretReadSourceContract';

type OwnedSpec = { namespace: string; excludeHelmReleases: boolean };
type PendingSourceSubscription = OwnedSpec & {
  candidates: SecretEvent[];
  consumers: Set<(event: SecretEvent) => void>;
};

const MAX_PENDING_SOURCE_EVENTS = 64;

const isHelmSecret = (item: any) => item?.type === 'helm.sh/release.v1';
const eventNamespace = (event: any) => event?.namespace ?? event?.resource?.metadata?.namespace ?? '';
const matchesNamespace = (specNamespace: string, namespace: string) => specNamespace === '' || specNamespace === namespace;

const projectDirectSecret = (resource: any) => ({
  metadata: {
    name: resource?.metadata?.name,
    namespace: resource?.metadata?.namespace,
    uid: resource?.metadata?.uid,
    creationTimestamp: resource?.metadata?.creationTimestamp,
  },
  type: resource?.type,
  dataKeys: typeof resource?.dataKeys === 'number' ? resource.dataKeys : Object.keys(resource?.data ?? {}).length,
});

const appendPendingEvent = (events: SecretEvent[], event: SecretEvent) => {
  if (events.length === MAX_PENDING_SOURCE_EVENTS) events.shift();
  events.push(event);
};

function sourceListeners(sourceKey: string, specs: Map<string, OwnedSpec>, accelerator: boolean) {
  const resourceCallbacks = new Set<(event: SecretEvent) => void>();
  const pendingSubscriptions = new Set<PendingSourceSubscription>();
  let cancelResourceSingle: (() => void) | null = null;
  let cancelResourceBatch: (() => void) | null = null;

  const taggedListener = (name: string, cb: (event: any) => void, accept: (event: any) => boolean) =>
    EventsOn(name, (event: any) => {
      if (accept(event)) cb({ ...event, sourceKey });
    });

  const emitOwnedResource = (event: any) => {
    if (event?.resourceType !== 'secrets') return;
    const declaredNamespace = event?.namespace;
    const resourceNamespace = event?.resource?.metadata?.namespace;
    if (declaredNamespace !== undefined && resourceNamespace !== undefined && declaredNamespace !== resourceNamespace) return;
    const namespace = eventNamespace(event);
    if (accelerator) {
      if (!['ADDED', 'MODIFIED', 'DELETED'].includes(event?.type) || !event?.resource?.metadata?.uid) return;
      const watcherSpecId = event?.watcherSpecId;
      if (!watcherSpecId) return;
      const spec = watcherSpecId ? specs.get(watcherSpecId) : undefined;
      const projected = {
        type: event?.type,
        resourceType: event?.resourceType,
        namespace,
        watcherSpecId,
        resource: projectDirectSecret(event?.resource),
        sourceKey,
      };
      if (spec && matchesNamespace(spec.namespace, namespace)) {
        if (spec.excludeHelmReleases && isHelmSecret(event?.resource)) return;
        for (const cb of resourceCallbacks) cb(projected);
        return;
      }
      for (const pending of pendingSubscriptions) {
        if (!matchesNamespace(pending.namespace, namespace)) continue;
        if (pending.excludeHelmReleases && isHelmSecret(event?.resource)) continue;
        appendPendingEvent(pending.candidates, projected);
      }
      return;
    }
    for (const [watcherSpecId, spec] of specs) {
      if (!matchesNamespace(spec.namespace, namespace)) continue;
      if (spec.excludeHelmReleases && isHelmSecret(event?.resource)) continue;
      const projected = {
        type: event.type,
        resourceType: event.resourceType,
        namespace,
        resource: projectDirectSecret(event.resource),
        watcherSpecId,
        sourceKey,
      };
      for (const cb of resourceCallbacks) cb(projected);
    }
  };

  const emitOwnedSignal = (event: any, cb: (event: any) => void) => {
    if (accelerator) {
      const watcherSpecId = event?.watcherSpecId;
      if (watcherSpecId && specs.has(watcherSpecId)) cb({ ...event, sourceKey });
      return;
    }
    if (event?.resourceType !== 'secrets') return;
    const namespace = eventNamespace(event);
    for (const [watcherSpecId, spec] of specs) {
      if (matchesNamespace(spec.namespace, namespace)) {
        cb({ ...event, namespace, watcherSpecId, sourceKey });
      }
    }
  };

  const listeners = {
    onProgress: (cb: (event: any) => void) => taggedListener('list-progress', cb, event => event?.resourceType === 'secrets'),
    onResource: (cb: (event: SecretEvent) => void) => {
      resourceCallbacks.add(cb);
      if (resourceCallbacks.size === 1) {
        cancelResourceSingle = EventsOn('resource-event', emitOwnedResource);
        cancelResourceBatch = EventsOn('resource-events-batch', (events: any) => {
          if (!Array.isArray(events)) return;
          for (const event of events) emitOwnedResource(event);
        });
      }
      return () => {
        resourceCallbacks.delete(cb);
        for (const pending of pendingSubscriptions) {
          pending.consumers.delete(cb);
          if (pending.consumers.size !== 0) continue;
          pending.candidates.length = 0;
          pendingSubscriptions.delete(pending);
        }
        if (resourceCallbacks.size !== 0) return;
        cancelResourceSingle?.();
        cancelResourceBatch?.();
        cancelResourceSingle = null;
        cancelResourceBatch = null;
      };
    },
    onStatus: (cb: (event: WatcherStatus) => void) => EventsOn('watcher-status', (event: any) => emitOwnedSignal(event, cb)),
    onError: (cb: (event: WatcherError) => void) => EventsOn('watcher-error', (event: any) => emitOwnedSignal(event, cb)),
    onConnected: (cb: (event: any) => void) => taggedListener('connected', cb, () => true),
  };

  return {
    listeners,
    beginPending(namespace: string, excludeHelmReleases: boolean) {
      const pending: PendingSourceSubscription = {
        namespace,
        excludeHelmReleases,
        candidates: [],
        consumers: new Set(resourceCallbacks),
      };
      if (pending.consumers.size !== 0) pendingSubscriptions.add(pending);
      return pending;
    },
    settlePending(pending: PendingSourceSubscription, watcherSpecId: string) {
      pendingSubscriptions.delete(pending);
      const candidates = pending.candidates.splice(0);
      if (!watcherSpecId) return;
      for (const event of candidates) {
        if (event.watcherSpecId !== watcherSpecId) continue;
        for (const cb of pending.consumers) {
          if (resourceCallbacks.has(cb)) cb(event);
        }
      }
    },
  };
}

function createDirectSecretReadSource(): SecretReadSource {
  const specs = new Map<string, OwnedSpec>();
  const sourceKey = 'direct-secrets';
  const listenerState = sourceListeners(sourceKey, specs, false);
  return {
    sourceKey,
    list: async (requestId, namespace, excludeHelmReleases) => {
      const rows = await ListSecretsMetadata(requestId, namespace);
      const normalizedRows = Array.isArray(rows) ? rows : [];
      return excludeHelmReleases ? normalizedRows.filter((row: any) => !isHelmSecret(row)) : normalizedRows;
    },
    cancelList: requestId => CancelListRequest(requestId),
    subscribe: async (namespace, excludeHelmReleases) => {
      const watcherSpecId = await SubscribeResourceWatcher('secrets', namespace);
      if (watcherSpecId) specs.set(watcherSpecId, { namespace, excludeHelmReleases });
      return watcherSpecId;
    },
    unsubscribe: async watcherSpecId => {
      specs.delete(watcherSpecId);
      return UnsubscribeWatcher(watcherSpecId);
    },
    getSecretData: GetSecretData,
    getSecretYaml: GetSecretYaml,
    ...listenerState.listeners,
  };
}

export const directSecretReadSource = createDirectSecretReadSource();

export function createAcceleratorSecretReadSource(sourceKey: string): SecretReadSource {
  if (!sourceKey.trim()) throw new Error('Secret read source key must be non-empty');
  const specs = new Map<string, OwnedSpec>();
  const listenerState = sourceListeners(sourceKey, specs, true);
  return {
    sourceKey,
    list: (requestId, namespace, excludeHelmReleases) =>
      ListAcceleratorSecretsMetadata(requestId, namespace, excludeHelmReleases),
    cancelList: requestId => CancelListRequest(requestId),
    subscribe: async (namespace, excludeHelmReleases) => {
      const pending = listenerState.beginPending(namespace, excludeHelmReleases);
      try {
        const result: any = await SubscribeSecretWatcher(namespace, excludeHelmReleases);
        const watcherSpecId = result?.watcherSpecId ?? result;
        if (watcherSpecId) specs.set(watcherSpecId, { namespace, excludeHelmReleases });
        listenerState.settlePending(pending, watcherSpecId);
        return watcherSpecId;
      } catch (error) {
        listenerState.settlePending(pending, '');
        throw error;
      }
    },
    unsubscribe: async watcherSpecId => {
      specs.delete(watcherSpecId);
      return UnsubscribeSecretWatcher(watcherSpecId);
    },
    getSecretData: GetSecretData,
    getSecretYaml: GetSecretYaml,
    ...listenerState.listeners,
  };
}

const SecretReadSourceContext = createContext<SecretReadSource>(directSecretReadSource);
export const SecretReadSourceProvider = SecretReadSourceContext.Provider;
export const useSecretReadSource = () => useContext(SecretReadSourceContext);
