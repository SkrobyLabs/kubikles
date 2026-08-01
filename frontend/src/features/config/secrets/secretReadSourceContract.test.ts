import { describe, expect, it } from 'vitest';
import type { SecretEvent, SecretListState, SecretReadSource, WatcherError, WatcherStatus } from './secretReadSourceContract';
import contractSource from './secretReadSourceContract.ts?raw';

describe('binding-free Secret read source contract', () => {
  it('has no runtime imports and preserves the settled type surface', () => {
    expect(contractSource).not.toMatch(/^import /m);
    expect(contractSource).not.toMatch(/wailsjs|runtime|context|provider/i);
    const event: SecretEvent = { type: 'ADDED' };
    const status: WatcherStatus = { status: 'connected' };
    const error: WatcherError = { code: 'resource_version_expired' };
    const state: SecretListState = { secrets: [], loading: false, error: null, loadingProgress: null };
    const sourceContract: SecretReadSource | null = null;
    expect({ event, status, error, state, sourceContract }).toMatchObject({ event: { type: 'ADDED' } });
  });
});
