/** @vitest-environment jsdom */
import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const bridge = vi.hoisted(() => ({
  GetHelmRelease: vi.fn(), GetHelmReleaseAllValues: vi.fn(), GetHelmReleaseHistory: vi.fn(), GetHelmReleaseValues: vi.fn(),
  RollbackHelmRelease: vi.fn(), UninstallHelmRelease: vi.fn(),
}));
const fixture = vi.hoisted(() => {
  let resourceListener: ((event: any) => void) | null = null;
  let statusListener: ((event: any) => void) | null = null;
  const disposeResource = vi.fn();
  const disposeStatus = vi.fn();
  const source = {
    sourceKey: 'integrated-source',
    listHelmReleaseMetadata: vi.fn(),
    cancelList: vi.fn(),
    subscribe: vi.fn().mockResolvedValue('helm-watch'),
    unsubscribe: vi.fn(),
    onResource: vi.fn((listener: (event: any) => void) => {
      resourceListener = listener;
      return disposeResource;
    }),
    onStatus: vi.fn((listener: (event: any) => void) => {
      statusListener = listener;
      return disposeStatus;
    }),
  };
  return {
    source, disposeResource, disposeStatus,
    emit: (event: any) => resourceListener?.(event),
    status: (event: any) => statusListener?.(event),
  };
});

vi.mock('wailsjs/go/main/App', () => bridge);
vi.mock('../context', () => ({ useK8s: () => ({ namespaces: ['team-a'], lastRefresh: 1 }) }));
vi.mock('~/features/config/secrets/secretReadSource', () => ({ useSecretReadSource: () => fixture.source }));

import { useHelmReleases } from './useHelmReleases';

const release = (status: string) => ({
  name: 'example', namespace: 'team-a', revision: 2, status,
  chart: 'example-chart', chartVersion: '1.2.3', appVersion: '4.5.6',
  updated: '2026-08-10T19:30:00Z', description: 'Ready',
});

describe('useHelmReleases projected storage watch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fixture.source.subscribe.mockResolvedValue('helm-watch');
    fixture.source.listHelmReleaseMetadata.mockResolvedValue([release('pending-install')]);
  });

  it('lists projected releases and reconciles only Helm Secret events', async () => {
    const hook = renderHook(() => useHelmReleases('ctx', ['team-a'], true));
    await waitFor(() => expect(hook.result.current.releases).toEqual([release('pending-install')]));
    expect(fixture.source.subscribe).toHaveBeenCalledWith('', false);
    expect(fixture.source.listHelmReleaseMetadata).toHaveBeenCalledTimes(1);
    expect(fixture.source.listHelmReleaseMetadata.mock.calls[0][1]).toBe('');
    act(() => fixture.emit({ type: 'MODIFIED', resource: { type: 'Opaque' } }));
    await new Promise(resolve => setTimeout(resolve, 200));
    expect(fixture.source.listHelmReleaseMetadata).toHaveBeenCalledTimes(1);

    fixture.source.listHelmReleaseMetadata.mockResolvedValue([release('deployed')]);
    act(() => fixture.emit({ type: 'MODIFIED', resource: { type: 'helm.sh/release.v1' } }));
    await waitFor(() => expect(hook.result.current.releases).toEqual([release('deployed')]));
    expect(fixture.source.listHelmReleaseMetadata).toHaveBeenCalledTimes(2);

    hook.unmount();
    expect(fixture.disposeResource).toHaveBeenCalledOnce();
    expect(fixture.disposeStatus).toHaveBeenCalledOnce();
    expect(fixture.source.unsubscribe).toHaveBeenCalledWith('helm-watch');
  });
});
