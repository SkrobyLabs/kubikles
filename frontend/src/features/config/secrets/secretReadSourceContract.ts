export type SecretEvent = {
  type: string;
  resourceType?: string;
  namespace?: string;
  resource?: any;
  watcherSpecId?: string;
  sourceKey?: string;
};

export type WatcherStatus = {
  watcherSpecId?: string;
  status?: string;
  resourceType?: string;
  namespace?: string;
  sourceKey?: string;
};

export type WatcherError = {
  watcherSpecId?: string;
  code?: string;
  recoverable?: boolean;
  resourceType?: string;
  namespace?: string;
  sourceKey?: string;
};

export interface SecretReadSource {
  sourceKey: string;
  list(requestId: string, namespace: string, excludeHelmReleases: boolean): Promise<any[]>;
  cancelList(requestId: string): Promise<any>;
  subscribe(namespace: string, excludeHelmReleases: boolean): Promise<string>;
  unsubscribe(watcherSpecId: string): Promise<any>;
  getSecretData(namespace: string, name: string): Promise<any[]>;
  getSecretYaml(namespace: string, name: string): Promise<string>;
  onProgress(cb: (event: any) => void): () => void;
  onResource(cb: (event: SecretEvent) => void): () => void;
  onStatus(cb: (event: WatcherStatus) => void): () => void;
  onError(cb: (event: WatcherError) => void): () => void;
  onConnected(cb: (event: any) => void): () => void;
}

export type SecretListState = {
  secrets: any[];
  loading: boolean;
  error: any;
  loadingProgress: { loaded: number; total: number } | null;
};
