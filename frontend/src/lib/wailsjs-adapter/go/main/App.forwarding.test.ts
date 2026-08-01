/** @vitest-environment jsdom */
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  ListAcceleratorSecretsMetadata,
  ListSecretsMetadata,
  SubscribeSecretWatcher,
  UnsubscribeSecretWatcher,
} from './App';

describe('Accelerator Secret adapter forwarding', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    delete (window as any).go;
  });

  it('forwards the exact server methods and request bodies through the real adapter', async () => {
    const fetch = vi.fn(async (_url: string, request: RequestInit) => ({
      ok: true,
      json: async () => ({ data: request.body }),
      text: async () => '',
    }));
    vi.stubGlobal('fetch', fetch);
    delete (window as any).go;

    await ListSecretsMetadata('direct-request', 'team-a');
    await ListAcceleratorSecretsMetadata('request-id', 'team-a', true);
    await SubscribeSecretWatcher('team-a', false);
    await UnsubscribeSecretWatcher('spec-id');

    expect(fetch.mock.calls.map((call: any[]) => JSON.parse(call[1].body))).toEqual([
      { method: 'ListSecretsMetadata', args: ['direct-request', 'team-a'] },
      { method: 'ListSecretsMetadata', args: ['request-id', 'team-a', true] },
      { method: 'SubscribeSecretWatcher', args: ['team-a', false] },
      { method: 'UnsubscribeSecretWatcher', args: ['spec-id'] },
    ]);
  });
});
