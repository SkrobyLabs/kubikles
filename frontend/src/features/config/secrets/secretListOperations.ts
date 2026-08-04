import Logger from '~/utils/Logger';
import type { SecretEvent, SecretReadSource } from './secretReadSource';

export type SecretListState = {
  secrets: any[];
  loading: boolean;
  error: any;
  loadingProgress: { loaded: number; total: number } | null;
};

export const normalizeSecretNamespaces = (selected: string | string[], all: string[]) => {
  const values = Array.isArray(selected) ? selected : [selected];
  if (values.length === 0) return [];
  if (values.includes('') || values.includes('*')) return [''];
  const named = [...new Set(values.filter(Boolean))].sort();
  const available = [...new Set(all.filter(Boolean))];
  if (available.length > 0 && available.every(namespace => named.includes(namespace))) return [''];
  return named;
};

export const applySecretEvent = (map: Map<string, any>, event: SecretEvent) => {
  const uid = event.resource?.metadata?.uid;
  if (!uid || !['ADDED', 'MODIFIED', 'DELETED'].includes(event.type)) return map;
  const next = new Map(map);
  if (event.type === 'DELETED') {
    next.delete(uid);
  } else if (!(event.type === 'MODIFIED' && event.resource?.metadata?.deletionTimestamp && !next.has(uid))) {
    next.set(uid, event.resource);
  }
  return next;
};

type ChildRun = {
  id: string;
  namespace: string;
  started: boolean;
  pending: boolean;
  rows: any[];
};

type ListRun = {
  token: number;
  active: boolean;
  remaining: number;
  children: ChildRun[];
  progress: Map<string, { loaded: number; total: number }>;
  buffer: SecretEvent[];
  error: any;
};

type ActiveConfiguration = {
  generation: number;
  active: boolean;
  source: SecretReadSource;
  namespaces: string[];
  excludeHelmReleases: boolean;
  specs: Map<string, string>;
  pendingSubscriptions: Map<number, { namespace: string; candidates: SecretEvent[] }>;
  disposers: Array<() => void>;
  subscriptionTasks: Set<Promise<void>>;
  subscriptionSettled: Set<number>;
  unsubscribed: Set<string>;
  run: ListRun | null;
  reconcileScheduled: boolean;
  listIntent: number;
  listTransition: Promise<void>;
};

let nextOperation = 0;
let nextListToken = 0;
const MAX_PENDING_SUBSCRIPTION_EVENTS = 64;

const cancellationError = (error: any) => {
  const value = `${error?.name ?? ''} ${error?.message ?? error ?? ''}`.toLowerCase();
  return value.includes('abort') || value.includes('cancel');
};

const assertSource = (source: SecretReadSource) => {
  if (!source.sourceKey?.trim()) throw new Error('Secret read source key must be non-empty');
};

const operationError = (operation: string, namespace: string) =>
  new Error(`Secret ${operation} failed for namespace ${namespace || 'all namespaces'}`);

const logOperationFailure = (event: 'subscription_failure' | 'list_failure', requestId: string, namespace: string) => {
  if (typeof window === 'undefined') return;
  Logger.error('Secret list operation failure', {
    event,
    requestId,
    namespace: namespace || 'all-namespaces',
  }, 'config');
};

const projectPendingSecretEvent = (event: SecretEvent): SecretEvent => ({
  type: event.type,
  resourceType: event.resourceType,
  namespace: event.namespace,
  watcherSpecId: event.watcherSpecId,
  sourceKey: event.sourceKey,
  resource: {
    metadata: {
      name: event.resource?.metadata?.name,
      namespace: event.resource?.metadata?.namespace,
      uid: event.resource?.metadata?.uid,
      creationTimestamp: event.resource?.metadata?.creationTimestamp,
    },
    type: event.resource?.type,
    dataKeys: typeof event.resource?.dataKeys === 'number' ? event.resource.dataKeys : 0,
  },
});

export class SecretListOperationController {
  private emit: (state: SecretListState) => void;
  private checkConnectionError: (error: unknown) => boolean;
  private generation = 0;
  private transitionIntent = 0;
  private transition: Promise<void> = Promise.resolve();
  private current: ActiveConfiguration | null = null;
  private rows = new Map<string, any>();
  private state: SecretListState = { secrets: [], loading: false, error: null, loadingProgress: null };

  constructor(
    source: SecretReadSource,
    emit: (state: SecretListState) => void,
    checkConnectionError: (error: unknown) => boolean = () => false,
  ) {
    assertSource(source);
    this.emit = emit;
    this.checkConnectionError = checkConnectionError;
  }

  private publish(patch: Partial<SecretListState>) {
    // A row commit gets a fresh snapshot, but progress-only publications must
    // retain the exact snapshot already published.  Consumers can therefore
    // distinguish list changes from progress changes by identity.
    const secrets = patch.secrets === undefined ? this.state.secrets : [...patch.secrets];
    this.state = { ...this.state, ...patch, secrets };
    this.emit({ ...this.state, secrets: this.state.secrets });
  }

  replace(source: SecretReadSource, namespaces: string[], excludeHelmReleases: boolean, enabled: boolean) {
    assertSource(source);
    const intent = ++this.transitionIntent;
    const old = this.current;
    if (old) {
      old.active = false;
      if (old.run) old.run.active = false;
      old.pendingSubscriptions.clear();
    }
    this.current = null;
    ++this.generation;
    // This is deliberately synchronous. Teardown can wait on an old watcher
    // unsubscribe, while the UI must never continue showing its old context.
    this.rows = new Map();
    this.publish({ secrets: [], loading: false, error: null, loadingProgress: null });

    this.transition = this.transition.then(async () => {
      if (old) await this.teardownConfiguration(old);
      if (intent !== this.transitionIntent) return;
      if (!enabled || namespaces.length === 0) {
        this.rows = new Map();
        this.publish({ secrets: [], loading: false, error: null, loadingProgress: null });
        return;
      }
      this.startConfiguration(source, namespaces, excludeHelmReleases);
    });
    return this.transition;
  }

  stop() {
    const intent = ++this.transitionIntent;
    const old = this.current;
    if (old) {
      old.active = false;
      if (old.run) old.run.active = false;
      old.pendingSubscriptions.clear();
    }
    this.current = null;
    ++this.generation;
    this.rows = new Map();
    this.publish({ secrets: [], loading: false, error: null, loadingProgress: null });
    this.transition = this.transition.then(async () => {
      if (old) await this.teardownConfiguration(old);
      if (intent === this.transitionIntent) {
        this.publish({ secrets: [], loading: false, error: null, loadingProgress: null });
      }
    });
    return this.transition;
  }

  reconcile() {
    const config = this.current;
    if (!config || !config.active || config.reconcileScheduled) return;
    config.reconcileScheduled = true;
    queueMicrotask(() => {
      config.reconcileScheduled = false;
      if (!this.isCurrent(config)) return;
      this.queueListReplacement(config);
    });
  }

  private isCurrent(config: ActiveConfiguration) {
    return config.active && this.current === config && config.generation === this.generation;
  }

  private startConfiguration(source: SecretReadSource, namespaces: string[], excludeHelmReleases: boolean) {
    const config: ActiveConfiguration = {
      generation: this.generation,
      active: true,
      source,
      namespaces: [...namespaces],
      excludeHelmReleases,
      specs: new Map(),
      pendingSubscriptions: new Map(),
      disposers: [],
      subscriptionTasks: new Set(),
      subscriptionSettled: new Set(),
      unsubscribed: new Set(),
      run: null,
      reconcileScheduled: false,
      listIntent: 0,
      listTransition: Promise.resolve(),
    };
    this.current = config;
    this.rows = new Map();
    this.publish({ secrets: [], loading: true, error: null, loadingProgress: null });

    config.disposers = [
      source.onProgress(event => this.onProgress(config, event)),
      source.onResource(event => this.onResource(config, event)),
      source.onStatus(event => this.onSignal(config, event)),
      source.onError(event => this.onSignal(config, event)),
      source.onConnected(event => {
        if (this.isCurrent(config) && event?.sourceKey === source.sourceKey && event?.resumed === true) this.reconcile();
      }),
    ];

    config.run = this.createListRun(config);
    config.namespaces.forEach((namespace, index) => this.startSubscription(config, index, namespace));
  }

  private createListRun(config: ActiveConfiguration): ListRun {
    const operation = ++nextOperation;
    return {
      token: ++nextListToken,
      active: true,
      remaining: config.namespaces.length,
      children: config.namespaces.map((namespace, index) => ({
        id: `secret-${operation}-${index}-${encodeURIComponent(namespace || 'all')}`,
        namespace,
        started: false,
        pending: false,
        rows: [],
      })),
      progress: new Map(),
      buffer: [],
      error: null,
    };
  }

  private startSubscription(config: ActiveConfiguration, index: number, namespace: string) {
    const requestId = config.run?.children[index]?.id ?? `secret-subscription-${config.generation}-${index}`;
    config.pendingSubscriptions.set(index, { namespace, candidates: [] });
    let subscribePromise: Promise<string>;
    try {
      subscribePromise = config.source.subscribe(namespace, config.excludeHelmReleases);
    } catch (error) {
      subscribePromise = Promise.reject(error);
    }
    let task: Promise<void>;
    task = subscribePromise.then(async watcherSpecId => {
      if (!this.isCurrent(config)) {
        config.pendingSubscriptions.delete(index);
        if (watcherSpecId) await this.unsubscribeOnce(config, watcherSpecId);
        return;
      }
      const pending = config.pendingSubscriptions.get(index);
      config.pendingSubscriptions.delete(index);
      if (watcherSpecId) {
        config.specs.set(watcherSpecId, namespace);
        for (const event of pending?.candidates ?? []) {
          if (event.watcherSpecId === watcherSpecId && this.owns(config, event)) this.applyOwnedResource(config, event);
        }
      }
    }).catch(error => {
      config.pendingSubscriptions.delete(index);
      if (this.isCurrent(config) && !cancellationError(error)) {
        logOperationFailure('subscription_failure', requestId, namespace);
        this.checkConnectionError(error);
      }
    }).finally(() => {
      config.subscriptionSettled.add(index);
      if (this.isCurrent(config) && config.run) this.startChild(config, config.run, index);
      config.subscriptionTasks.delete(task);
    });
    config.subscriptionTasks.add(task);
  }

  private startChild(config: ActiveConfiguration, run: ListRun, index: number) {
    if (!this.isCurrent(config) || config.run !== run || !run.active) return;
    const child = run.children[index];
    if (!child || child.started) return;
    child.started = true;
    child.pending = true;
    let listPromise: Promise<any[]>;
    try {
      listPromise = config.source.list(child.id, child.namespace, config.excludeHelmReleases);
    } catch (error) {
      listPromise = Promise.reject(error);
    }
    listPromise.then(rows => {
      if (this.isCurrent(config) && config.run === run && run.active && child.pending) {
        child.rows = Array.isArray(rows) ? rows : [];
      }
    }).catch(error => {
      if (!this.isCurrent(config) || config.run !== run || !run.active || !child.pending || cancellationError(error)) return;
      const diagnostic = operationError('list', child.namespace);
      if (child.namespace !== '') logOperationFailure('list_failure', child.id, child.namespace);
      this.checkConnectionError(error);
      if (child.namespace === '') run.error = diagnostic;
    }).finally(() => this.settleChild(config, run, child));
  }

  private settleChild(config: ActiveConfiguration, run: ListRun, child: ChildRun) {
    if (!this.isCurrent(config) || config.run !== run || !run.active || !child.pending) return;
    child.pending = false;
    run.progress.delete(child.id);
    run.remaining -= 1;
    this.publishProgress(run);
    if (run.remaining !== 0) return;

    let rows = new Map<string, any>();
    for (const current of run.children) {
      for (const row of current.rows) {
        const uid = row?.metadata?.uid;
        if (uid) rows.set(uid, row);
      }
    }
    for (const event of run.buffer) rows = applySecretEvent(rows, event);
    run.buffer = [];
    this.rows = rows;
    this.publish({
      secrets: [...rows.values()],
      loading: false,
      error: run.error,
      loadingProgress: null,
    });
  }

  private onProgress(config: ActiveConfiguration, event: any) {
    const run = config.run;
    if (!this.isCurrent(config) || !run?.active || event?.sourceKey !== config.source.sourceKey) return;
    const child = run.children.find(value => value.id === event?.requestId);
    if (!child?.pending) return;
    run.progress.set(child.id, { loaded: Number(event.loaded) || 0, total: Number(event.total) || 0 });
    this.publishProgress(run);
  }

  private publishProgress(run: ListRun) {
    if (!run.active) return;
    let loaded = 0;
    let total = 0;
    let found = false;
    for (const child of run.children) {
      if (!child.pending) continue;
      const progress = run.progress.get(child.id);
      if (!progress) continue;
      found = true;
      loaded += progress.loaded;
      total += progress.total;
    }
    this.publish({ loadingProgress: found ? { loaded, total } : null });
  }

  private owns(config: ActiveConfiguration, event: any) {
    const watcherSpecId = event?.watcherSpecId;
    if (!watcherSpecId) return false;
    const expectedNamespace = config.specs.get(watcherSpecId);
    if (expectedNamespace === undefined) return false;
    const declaredNamespace = event?.namespace;
    const resourceNamespace = event?.resource?.metadata?.namespace;
    if (declaredNamespace !== undefined && resourceNamespace !== undefined && declaredNamespace !== resourceNamespace) return false;
    if (declaredNamespace === undefined && resourceNamespace === undefined) return true;
    const namespace = declaredNamespace ?? resourceNamespace ?? '';
    return expectedNamespace === '' || expectedNamespace === namespace;
  }

  private onResource(config: ActiveConfiguration, event: SecretEvent) {
    const run = config.run;
    if (!this.isCurrent(config) || !run?.active || event?.sourceKey !== config.source.sourceKey) return;
    if (event?.resourceType !== 'secrets') return;
    if (!['ADDED', 'MODIFIED', 'DELETED'].includes(event.type) || !event.resource?.metadata?.uid) return;
    if (config.excludeHelmReleases && event.resource?.type === 'helm.sh/release.v1') return;
    if (this.owns(config, event)) {
      this.applyOwnedResource(config, event);
      return;
    }
    if (!event.watcherSpecId) return;
    const declaredNamespace = event?.namespace;
    const resourceNamespace = event?.resource?.metadata?.namespace;
    if (declaredNamespace !== undefined && resourceNamespace !== undefined && declaredNamespace !== resourceNamespace) return;
    const namespace = declaredNamespace ?? resourceNamespace;
    for (const pending of config.pendingSubscriptions.values()) {
      if (pending.namespace !== '' && pending.namespace !== namespace) continue;
      if (pending.candidates.length === MAX_PENDING_SUBSCRIPTION_EVENTS) pending.candidates.shift();
      pending.candidates.push(projectPendingSecretEvent(event));
    }
  }

  private applyOwnedResource(config: ActiveConfiguration, event: SecretEvent) {
    const run = config.run;
    if (!this.isCurrent(config) || !run?.active || !this.owns(config, event)) return;
    if (run.remaining > 0) {
      run.buffer.push(event);
      return;
    }
    this.rows = applySecretEvent(this.rows, event);
    this.publish({ secrets: [...this.rows.values()] });
  }

  private onSignal(config: ActiveConfiguration, event: any) {
    if (!this.isCurrent(config) || event?.sourceKey !== config.source.sourceKey || !this.owns(config, event)) return;
    if (event?.status === 'reconnecting' || event?.code === 'resource_version_expired') this.reconcile();
  }

  private queueListReplacement(config: ActiveConfiguration) {
    const intent = ++config.listIntent;
    const old = config.run;
    if (old) old.active = false;
    config.run = null;
    this.publish({ loading: false, loadingProgress: null });
    config.listTransition = config.listTransition.then(async () => {
      if (old) await this.cancelRun(config, old);
      if (!this.isCurrent(config) || intent !== config.listIntent) return;
      const run = this.createListRun(config);
      config.run = run;
      this.publish({ loading: true, error: null, loadingProgress: null });
      for (const index of config.subscriptionSettled) this.startChild(config, run, index);
    });
  }

  private async cancelRun(config: ActiveConfiguration, run: ListRun) {
    run.active = false;
    const pending = run.children.filter(child => child.started && child.pending);
    for (const child of pending) child.pending = false;
    run.progress.clear();
    await Promise.allSettled(pending.map(child => config.source.cancelList(child.id)));
  }

  private async unsubscribeOnce(config: ActiveConfiguration, watcherSpecId: string) {
    if (!watcherSpecId || config.unsubscribed.has(watcherSpecId)) return;
    config.unsubscribed.add(watcherSpecId);
    await config.source.unsubscribe(watcherSpecId);
  }

  private async teardownConfiguration(config: ActiveConfiguration) {
    config.active = false;
    config.pendingSubscriptions.clear();
    config.disposers.splice(0).forEach(dispose => dispose());
    if (config.run) await this.cancelRun(config, config.run);
    await config.listTransition;
    await Promise.allSettled([...config.subscriptionTasks]);
    const owned = [...config.specs.keys()];
    config.specs.clear();
    await Promise.allSettled(owned.map(watcherSpecId => this.unsubscribeOnce(config, watcherSpecId)));
  }
}
