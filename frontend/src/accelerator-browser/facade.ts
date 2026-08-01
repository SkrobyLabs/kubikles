export type AcceleratorSecretSummary = {
  metadata: {
    name: string;
    namespace: string;
    uid: string;
    creationTimestamp: string;
  };
  type: string;
  dataKeys: number;
};

export type AcceleratorSecretDataEntry = {
  key: string;
  value: string;
  base64Value: string;
  isBinary: boolean;
  source: 'data';
  encoding: 'text' | 'base64';
};

export type AcceleratorBrowserEvent =
  | { type: 'resource-event'; data: { type: 'ADDED' | 'MODIFIED' | 'DELETED'; resourceType: 'secrets'; namespace: string; watcherSpecId: string; resource: AcceleratorSecretSummary } }
  | { type: 'watcher-status'; data: { watcherSpecId: string; status: 'connected' | 'reconnecting' } }
  | { type: 'watcher-error'; data: { watcherSpecId: string; code: 'watch_unavailable' | 'resource_version_expired' | 'malformed_watch_event'; recoverable: boolean } }
  | { type: 'connected'; data: { resumed: boolean } }
  | { type: 'list-progress'; data: { resourceType: string; requestId: string; loaded: number; total: number } }
  | { type: 'terminal' };

export interface AcceleratorBrowserFacade {
  ListSecretsMetadata(requestId: string, namespace: string, excludeHelmReleases: boolean): Promise<AcceleratorSecretSummary[]>;
  GetSecretData(namespace: string, name: string): Promise<AcceleratorSecretDataEntry[]>;
  GetSecretYaml(namespace: string, name: string): Promise<string>;
  CancelListRequest(requestId: string): Promise<unknown>;
  SubscribeSecretWatcher(namespace: string, excludeHelmReleases: boolean): Promise<{ watcherSpecId: string } | string>;
  UnsubscribeSecretWatcher(watcherSpecId: string): Promise<unknown>;
  events: { subscribe(callback: (event: AcceleratorBrowserEvent) => void): () => void };
  close(): Promise<unknown> | unknown;
}

export function isAcceleratorBrowserFacade(value: unknown): value is AcceleratorBrowserFacade {
  if (!value || typeof value !== 'object') return false;
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== Object.prototype && prototype !== null) return false;
  const expected = ['CancelListRequest', 'GetSecretData', 'GetSecretYaml', 'ListSecretsMetadata', 'SubscribeSecretWatcher', 'UnsubscribeSecretWatcher', 'close', 'events'];
  const keys = Reflect.ownKeys(value);
  if (keys.some(key => typeof key === 'symbol') || keys.filter((key): key is string => typeof key === 'string').sort().join('\0') !== expected.join('\0')) return false;
  const operationNames = expected.slice(0, -1);
  if (!operationNames.every(name => typeof Object.getOwnPropertyDescriptor(value, name)?.value === 'function')) return false;
  const events = Object.getOwnPropertyDescriptor(value, 'events')?.value;
  if (!events || typeof events !== 'object') return false;
  const eventsPrototype = Object.getPrototypeOf(events);
  if (eventsPrototype !== Object.prototype && eventsPrototype !== null) return false;
  const eventKeys = Reflect.ownKeys(events);
  return eventKeys.length === 1 && eventKeys[0] === 'subscribe' && typeof Object.getOwnPropertyDescriptor(events, 'subscribe')?.value === 'function';
}
