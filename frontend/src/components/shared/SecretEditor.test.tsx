/** @vitest-environment jsdom */
import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SecretReadSource } from '~/features/config/secrets/secretReadSource';
import { SecretReadSourceProvider } from '~/features/config/secrets/secretReadSource';
import SecretEditor from './SecretEditor';

const api = vi.hoisted(() => ({
  UpdateSecretYaml: vi.fn(), UpdateSecretData: vi.fn(), GetAllCertificateInfo: vi.fn(), SaveDataEntryValue: vi.fn(),
  CancelListRequest: vi.fn(), GetSecretData: vi.fn(), GetSecretYaml: vi.fn(), ListAcceleratorSecretsMetadata: vi.fn(), ListSecretsMetadata: vi.fn(),
  SubscribeResourceWatcher: vi.fn(), SubscribeSecretWatcher: vi.fn(), UnsubscribeSecretWatcher: vi.fn(), UnsubscribeWatcher: vi.fn(),
}));
const notifications = vi.hoisted(() => ({ add: vi.fn() }));
vi.mock('wailsjs/go/main/App', () => api);
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(() => () => {}) }));
vi.mock('~/context', () => ({
  useK8s: () => ({ currentContext: 'ctx' }),
  useNotification: () => ({ addNotification: notifications.add }),
}));
vi.mock('@monaco-editor/react', () => ({ default: (props: any) => <textarea aria-label="yaml-editor" value={props.value} onChange={event => props.onChange(event.target.value)} /> }));
vi.mock('./CertificateModal', () => ({ default: () => null }));
vi.mock('./certificateBadge', () => ({ CertificateBadge: () => null, makeCertificateCacheEntry: (value: any) => value }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: any) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

const makeSource = (key: string): SecretReadSource => ({
  sourceKey: key,
  list: vi.fn(), cancelList: vi.fn(), subscribe: vi.fn(), unsubscribe: vi.fn(),
  getSecretData: vi.fn(), getSecretYaml: vi.fn(),
  onProgress: vi.fn(() => () => {}), onResource: vi.fn(() => () => {}), onStatus: vi.fn(() => () => {}), onError: vi.fn(() => () => {}), onConnected: vi.fn(() => () => {}),
} as any);

describe('SecretEditor source reads', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.UpdateSecretYaml.mockResolvedValue(undefined);
    api.UpdateSecretData.mockResolvedValue(undefined);
    api.GetAllCertificateInfo.mockResolvedValue([]);
  });

  it('keeps initial and post-save reads exact and concurrent while mutations stay direct', async () => {
    const source = makeSource('source-a');
    const exactYaml = `apiVersion: v1
kind: Secret
metadata:
  name: secret
  namespace: a
  resourceVersion: "17"
  uid: uid-secret
  labels:
    owner: exact-source
  annotations:
    note: preserved
  finalizers:
    - example.test/finalizer
type: Opaque
data:
  token: cGxhaW4=
`;
    const yamlInitial = deferred<string>();
    const dataInitial = deferred<any[]>();
    (source.getSecretYaml as any).mockReturnValueOnce(yamlInitial.promise);
    (source.getSecretData as any).mockReturnValueOnce(dataInitial.promise);
    render(<SecretReadSourceProvider value={source}><SecretEditor namespace="a" resourceName="secret" onClose={vi.fn()} /></SecretReadSourceProvider>);
    expect(source.getSecretYaml).toHaveBeenCalledWith('a', 'secret');
    expect(source.getSecretData).toHaveBeenCalledWith('a', 'secret');
    expect(screen.getByText(/Loading secret/)).toBeTruthy();
    yamlInitial.resolve(exactYaml);
    dataInitial.resolve([{ key: 'token', value: 'plain', base64Value: 'cGxhaW4=', isBinary: false, source: 'data', encoding: 'text' }]);
    await waitFor(() => expect((screen.getByLabelText('yaml-editor') as HTMLTextAreaElement).value).toBe(exactYaml));

    (source.getSecretData as any).mockResolvedValueOnce([{ key: 'after-yaml', value: 'v' }]);
    fireEvent.change(screen.getByLabelText('yaml-editor'), { target: { value: 'updated: yaml' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.UpdateSecretYaml).toHaveBeenCalledWith('a', 'secret', 'updated: yaml'));
    await waitFor(() => expect(source.getSecretData).toHaveBeenCalledTimes(2));
    expect(api.UpdateSecretData).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Key-Value' }));
    (source.getSecretYaml as any).mockResolvedValueOnce('after: keyvalue');
    (source.getSecretData as any).mockResolvedValueOnce([{ key: 'after-keyvalue', value: 'v' }]);
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.UpdateSecretData).toHaveBeenCalledOnce());
    await waitFor(() => expect(source.getSecretYaml).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(source.getSecretData).toHaveBeenCalledTimes(3));
    expect(api.UpdateSecretData.mock.calls[0][0]).toBe('a');
    expect(api.UpdateSecretData.mock.calls[0][1]).toBe('secret');
    expect(notifications.add).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    expect(api.SaveDataEntryValue).not.toHaveBeenCalled();
  });

  it('fences stale source and identity reads from committing or finalizing', async () => {
    const sourceA = makeSource('source-a');
    const sourceB = makeSource('source-b');
    const yamlA = deferred<string>();
    const dataA = deferred<any[]>();
    (sourceA.getSecretYaml as any).mockReturnValue(yamlA.promise);
    (sourceA.getSecretData as any).mockReturnValue(dataA.promise);
    (sourceB.getSecretYaml as any).mockResolvedValue('fresh: yaml');
    (sourceB.getSecretData as any).mockResolvedValue([]);
    const close = vi.fn();
    const { rerender, unmount } = render(
      <SecretReadSourceProvider value={sourceA}><SecretEditor namespace="a" resourceName="old" onClose={close} /></SecretReadSourceProvider>,
    );
    rerender(<SecretReadSourceProvider value={sourceB}><SecretEditor namespace="b" resourceName="fresh" onClose={close} /></SecretReadSourceProvider>);
    await waitFor(() => expect((screen.getByLabelText('yaml-editor') as HTMLTextAreaElement).value).toBe('fresh: yaml'));
    yamlA.resolve('stale: yaml');
    dataA.resolve([{ key: 'stale' }]);
    await Promise.resolve();
    expect((screen.getByLabelText('yaml-editor') as HTMLTextAreaElement).value).toBe('fresh: yaml');
    unmount();
    expect(notifications.add).not.toHaveBeenCalledWith(expect.objectContaining({ message: expect.stringContaining('stale') }));
  });

  it('fences YAML-save data refresh after the source identity changes', async () => {
    const sourceA = makeSource('source-a');
    const sourceB = makeSource('source-b');
    (sourceA.getSecretYaml as any).mockResolvedValue('old: yaml');
    (sourceA.getSecretData as any).mockResolvedValueOnce([]);
    const refreshedData = deferred<any[]>();
    (sourceA.getSecretData as any).mockReturnValueOnce(refreshedData.promise);
    (sourceB.getSecretYaml as any).mockResolvedValue('fresh: yaml');
    (sourceB.getSecretData as any).mockResolvedValue([]);
    const { rerender } = render(<SecretReadSourceProvider value={sourceA}><SecretEditor namespace="a" resourceName="old" onClose={vi.fn()} /></SecretReadSourceProvider>);
    await waitFor(() => expect((screen.getByLabelText('yaml-editor') as HTMLTextAreaElement).value).toBe('old: yaml'));
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.UpdateSecretYaml).toHaveBeenCalled());
    await waitFor(() => expect(sourceA.getSecretData).toHaveBeenCalledTimes(2));
    rerender(<SecretReadSourceProvider value={sourceB}><SecretEditor namespace="b" resourceName="fresh" onClose={vi.fn()} /></SecretReadSourceProvider>);
    await waitFor(() => expect((screen.getByLabelText('yaml-editor') as HTMLTextAreaElement).value).toBe('fresh: yaml'));
    refreshedData.resolve([{ key: 'stale' }]);
    await Promise.resolve();
    expect(screen.queryByText('stale')).toBeNull();
  });

  it('fences key-value-save YAML and data refresh after the source identity changes', async () => {
    const sourceA = makeSource('source-a');
    const sourceB = makeSource('source-b');
    (sourceA.getSecretYaml as any).mockResolvedValueOnce('old: yaml');
    (sourceA.getSecretData as any).mockResolvedValueOnce([]);
    const refreshedYaml = deferred<string>();
    const refreshedData = deferred<any[]>();
    (sourceA.getSecretYaml as any).mockReturnValueOnce(refreshedYaml.promise);
    (sourceA.getSecretData as any).mockReturnValueOnce(refreshedData.promise);
    (sourceB.getSecretYaml as any).mockResolvedValue('fresh: yaml');
    (sourceB.getSecretData as any).mockResolvedValue([]);
    const { rerender } = render(<SecretReadSourceProvider value={sourceA}><SecretEditor namespace="a" resourceName="old" onClose={vi.fn()} initialMode="keyvalue" /></SecretReadSourceProvider>);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.UpdateSecretData).toHaveBeenCalled());
    rerender(<SecretReadSourceProvider value={sourceB}><SecretEditor namespace="b" resourceName="fresh" onClose={vi.fn()} initialMode="keyvalue" /></SecretReadSourceProvider>);
    await waitFor(() => expect(screen.getByRole('button', { name: 'YAML' })).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: 'YAML' }));
    await waitFor(() => expect((screen.getByLabelText('yaml-editor') as HTMLTextAreaElement).value).toBe('fresh: yaml'));
    refreshedYaml.resolve('stale: yaml');
    refreshedData.resolve([{ key: 'stale' }]);
    await Promise.resolve();
    expect((screen.getByLabelText('yaml-editor') as HTMLTextAreaElement).value).toBe('fresh: yaml');
  });
});
