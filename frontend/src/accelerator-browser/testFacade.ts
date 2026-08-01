import { vi } from 'vitest';
import type { AcceleratorBrowserEvent, AcceleratorBrowserFacade } from './facade';

export function createTestFacade() {
  const subscribers = new Set<(event: AcceleratorBrowserEvent) => void>();
  const facade: AcceleratorBrowserFacade = {
    ListSecretsMetadata: vi.fn(async () => []),
    GetSecretData: vi.fn(async () => []),
    GetSecretYaml: vi.fn(async () => ''),
    CancelListRequest: vi.fn(async () => undefined),
    SubscribeSecretWatcher: vi.fn(async () => ({ watcherSpecId: 'spec-all' })),
    UnsubscribeSecretWatcher: vi.fn(async () => undefined),
    events: { subscribe: vi.fn(callback => { subscribers.add(callback); return () => subscribers.delete(callback); }) },
    close: vi.fn(async () => undefined),
  };
  return {
    facade,
    subscribers,
    emit(event: AcceleratorBrowserEvent) { for (const callback of [...subscribers]) callback(event); },
  };
}
