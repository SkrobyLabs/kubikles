import React, { createContext, useCallback, useContext, useEffect, useLayoutEffect, useRef, useState } from 'react';
import {
  CancelIntegratedSecretListRequest,
  CancelListRequest,
  GetIntegratedSecretData,
  GetIntegratedSecretYaml,
  GetSecretData,
  GetSecretYaml,
  ListIntegratedSecretsMetadata,
  ListIntegratedHelmReleaseMetadata,
  ListHelmReleaseMetadata,
  ListSecretsMetadata,
  ReleaseIntegratedSecretReads,
  RetainIntegratedSecretReads,
  SubscribeIntegratedSecretWatcher,
  SubscribeResourceWatcher,
  UnsubscribeIntegratedSecretWatcher,
  UnsubscribeWatcher,
} from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { isInServerMode } from '~/lib/wailsjs-adapter/runtime/runtime';
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
    listHelmReleaseMetadata: (requestId, namespace) => ListHelmReleaseMetadata(requestId, namespace),
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

const INTEGRATED_READY_EVENT = 'accelerator:secret-source-ready';
const INTEGRATED_UNAVAILABLE_EVENT = 'accelerator:secret-source-unavailable';
const INTEGRATED_RESOURCE_EVENT = 'accelerator:secret-resource';
const INTEGRATED_STATUS_EVENT = 'accelerator:secret-watcher-status';
const INTEGRATED_ERROR_EVENT = 'accelerator:secret-watcher-error';
const SOURCE_TOKEN = /^s\.[A-Za-z0-9_-]{22}\.[0-9a-f]{16}$/;

const noListener = () => () => {};
const validSourceToken = (value: unknown): value is string => typeof value === 'string' && SOURCE_TOKEN.test(value);
const exactControlToken = (event: any) => {
  if (!event || Object.keys(event).length !== 1 || !validSourceToken(event.sourceToken)) return '';
  return event.sourceToken;
};

const projectIntegratedResource = (event: any, sourceKey: string): SecretEvent | null => {
  if (event?.sourceToken !== sourceKey) return null;
  const metadata = event?.resource?.metadata;
  if (event.resourceType !== 'secrets' || !['ADDED', 'MODIFIED', 'DELETED'].includes(event.type) ||
      typeof event.watcherSpecId !== 'string' || !event.watcherSpecId || !metadata?.uid ||
      event.namespace !== metadata.namespace) return null;
  return {
    type: event.type,
    resourceType: 'secrets',
    namespace: event.namespace,
    watcherSpecId: event.watcherSpecId,
    sourceKey,
    resource: {
      metadata: {
        name: metadata.name,
        namespace: metadata.namespace,
        uid: metadata.uid,
        creationTimestamp: metadata.creationTimestamp,
      },
      type: event.resource.type,
      dataKeys: typeof event.resource.dataKeys === 'number' ? event.resource.dataKeys : 0,
    },
  };
};

export function createIntegratedAcceleratorSecretReadSource(sourceToken: string): SecretReadSource {
  if (!validSourceToken(sourceToken)) throw new Error('Secret read source token is unavailable');
  const specs = new Set<string>();
  const resourceCallbacks = new Set<(event: SecretEvent) => void>();
  const pending = new Set<{ candidates: SecretEvent[]; consumers: Set<(event: SecretEvent) => void> }>();
  let disposeResource: (() => void) | null = null;

  const deliverResource = (raw: any) => {
    if (raw?.sourceToken !== sourceToken) return;
    const event = projectIntegratedResource(raw, sourceToken);
    if (!event) return;
    if (specs.has(event.watcherSpecId ?? '')) {
      for (const callback of resourceCallbacks) callback(event);
      return;
    }
    for (const candidate of pending) appendPendingEvent(candidate.candidates, event);
  };

  return {
    sourceKey: sourceToken,
    list: (requestId, namespace, excludeHelmReleases) =>
      ListIntegratedSecretsMetadata(sourceToken, requestId, namespace, excludeHelmReleases),
    listHelmReleaseMetadata: (requestId, namespace) =>
      ListIntegratedHelmReleaseMetadata(sourceToken, requestId, namespace),
    cancelList: requestId => CancelIntegratedSecretListRequest(sourceToken, requestId),
    subscribe: async (namespace, excludeHelmReleases) => {
      const candidate = { candidates: [] as SecretEvent[], consumers: new Set(resourceCallbacks) };
      if (candidate.consumers.size !== 0) pending.add(candidate);
      try {
        const watcherSpecId = await SubscribeIntegratedSecretWatcher(sourceToken, namespace, excludeHelmReleases);
        pending.delete(candidate);
        if (!watcherSpecId) return '';
        specs.add(watcherSpecId);
        for (const event of candidate.candidates) {
          if (event.watcherSpecId !== watcherSpecId) continue;
          for (const callback of candidate.consumers) if (resourceCallbacks.has(callback)) callback(event);
        }
        return watcherSpecId;
      } catch (error) {
        pending.delete(candidate);
        candidate.candidates.length = 0;
        throw error;
      }
    },
    unsubscribe: async watcherSpecId => {
      specs.delete(watcherSpecId);
      return UnsubscribeIntegratedSecretWatcher(sourceToken, watcherSpecId);
    },
    getSecretData: (namespace, name) => GetIntegratedSecretData(sourceToken, namespace, name),
    getSecretYaml: (namespace, name) => GetIntegratedSecretYaml(sourceToken, namespace, name),
    onProgress: noListener,
    onConnected: noListener,
    onResource: callback => {
      resourceCallbacks.add(callback);
      if (resourceCallbacks.size === 1) disposeResource = EventsOn(INTEGRATED_RESOURCE_EVENT, deliverResource);
      return () => {
        resourceCallbacks.delete(callback);
        for (const candidate of pending) candidate.consumers.delete(callback);
        if (resourceCallbacks.size !== 0) return;
        disposeResource?.();
        disposeResource = null;
      };
    },
    onStatus: callback => EventsOn(INTEGRATED_STATUS_EVENT, (event: any) => {
      if (event?.sourceToken !== sourceToken || !specs.has(event.watcherSpecId)) return;
      callback({ watcherSpecId: event.watcherSpecId, status: event.status, sourceKey: sourceToken });
    }),
    onError: callback => EventsOn(INTEGRATED_ERROR_EVENT, (event: any) => {
      if (event?.sourceToken !== sourceToken || !specs.has(event.watcherSpecId)) return;
      callback({ watcherSpecId: event.watcherSpecId, code: event.code, recoverable: event.recoverable === true, sourceKey: sourceToken });
    }),
  };
}

const SecretReadSourceContext = createContext<SecretReadSource>(directSecretReadSource);

export function SecretReadSourceProvider({ value, children }: { value: SecretReadSource; children: React.ReactNode }) {
  return <SecretReadSourceContext.Provider value={value}>{children}</SecretReadSourceContext.Provider>;
}

type IntegratedSourceTransition = 'stable' | 'awaitingDirectCommit';
type IntegratedSourceState = {
  source: SecretReadSource;
  transition: IntegratedSourceTransition;
};

export function IntegratedSecretReadSourceProvider({ children }: { children: React.ReactNode }) {
  const [sourceState, setSourceState] = useState<IntegratedSourceState>({ source: directSecretReadSource, transition: 'stable' });
  const transition = useRef<IntegratedSourceTransition>('stable');
  const currentOrPendingToken = useRef('');
  const lifecycle = useRef<Promise<void>>(Promise.resolve());

  useLayoutEffect(() => {
    const disposeReady = EventsOn(INTEGRATED_READY_EVENT, (event: any) => {
      const token = exactControlToken(event);
      if (!token || currentOrPendingToken.current) return;
      currentOrPendingToken.current = token;
      if (transition.current === 'awaitingDirectCommit') return;
      setSourceState({ source: createIntegratedAcceleratorSecretReadSource(token), transition: 'stable' });
    });
    const disposeUnavailable = EventsOn(INTEGRATED_UNAVAILABLE_EVENT, (event: any) => {
      const token = exactControlToken(event);
      if (!token || currentOrPendingToken.current !== token) return;
      currentOrPendingToken.current = '';
      if (transition.current === 'awaitingDirectCommit') return;
      transition.current = 'awaitingDirectCommit';
      setSourceState({ source: directSecretReadSource, transition: 'awaitingDirectCommit' });
    });
    return () => {
      transition.current = 'stable';
      currentOrPendingToken.current = '';
      disposeReady();
      disposeUnavailable();
    };
  }, []);

  useEffect(() => {
    if (sourceState.transition !== 'awaitingDirectCommit' || transition.current !== 'awaitingDirectCommit') return;
    // Descendant passive effects observe and replace Direct before this parent
    // passive effect applies the sole pending ready token in a later commit.
    transition.current = 'stable';
    const pendingToken = currentOrPendingToken.current;
    const pendingSource = pendingToken ? createIntegratedAcceleratorSecretReadSource(pendingToken) : directSecretReadSource;
    setSourceState(current => current.transition === 'awaitingDirectCommit'
      ? { source: pendingSource, transition: 'stable' }
      : current);
  }, [sourceState]);

  const retain = useCallback(() => {
    let active = true;
    let retained = false;
    lifecycle.current = lifecycle.current.then(async () => {
      if (!active) return;
      try {
        await RetainIntegratedSecretReads();
        retained = true;
      } catch {
        return;
      }
      if (!active && retained) {
        retained = false;
        try { await ReleaseIntegratedSecretReads(); } catch { /* direct remains authoritative */ }
      }
    });
    return () => {
      active = false;
      lifecycle.current = lifecycle.current.then(async () => {
        if (!retained) return;
        retained = false;
        try { await ReleaseIntegratedSecretReads(); } catch { /* backend teardown remains authoritative */ }
      });
    };
  }, []);

  // The Accelerator session belongs to the active Kubikles context, not to a
  // particular Secret view. Navigation may unsubscribe view-specific watchers,
  // but only a context switch or provider shutdown releases the backend route.
  useEffect(() => retain(), [retain]);

  return <SecretReadSourceContext.Provider value={sourceState.source}>{children}</SecretReadSourceContext.Provider>;
}

export function RuntimeSecretReadSourceProvider({ children }: { children: React.ReactNode }) {
  if (isInServerMode()) {
    return <SecretReadSourceProvider value={directSecretReadSource}>{children}</SecretReadSourceProvider>;
  }
  return <IntegratedSecretReadSourceProvider>{children}</IntegratedSecretReadSourceProvider>;
}

export const useSecretReadSource = () => {
  return useContext(SecretReadSourceContext);
};
