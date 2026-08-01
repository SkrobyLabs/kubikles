/** @vitest-environment jsdom */
import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SecretReadSource } from './secretReadSource';
import { SecretReadSourceProvider } from './secretReadSource';
import SecretList from './SecretList';

const mocks = vi.hoisted(() => ({
  resourceProps: null as any,
  list: vi.fn(),
  setSelectedNamespaces: vi.fn(),
  editKeyValue: vi.fn(),
  editYaml: vi.fn(),
  dependencies: vi.fn(),
  openBulkDelete: vi.fn(),
}));
vi.mock('./useSecretListOperations', () => ({ useSecretListOperations: (...args: any[]) => mocks.list(...args) }));
vi.mock('~/context', () => ({ useK8s: () => ({ currentContext: 'ctx', selectedNamespaces: ['a'], setSelectedNamespaces: mocks.setSelectedNamespaces, namespaces: ['a', 'b'] }) }));
vi.mock('~/components/shared/ResourceList', () => ({ default: (props: any) => { mocks.resourceProps = props; return props.customHeaderActions; } }));
vi.mock('~/components/shared/BulkActionModal', () => ({ default: () => null }));
vi.mock('~/hooks/useSelection', () => ({ useSelection: () => ({ selectedCount: 0 }) }));
vi.mock('~/hooks/useBulkActions', () => ({ useBulkActions: () => ({ bulkModalProps: {}, openBulkDelete: mocks.openBulkDelete, exportYaml: vi.fn() }) }));
vi.mock('./useSecretActions', () => ({ useSecretActions: () => ({ handleEditYaml: mocks.editYaml, handleEditKeyValue: mocks.editKeyValue, handleShowDependencies: mocks.dependencies }) }));
vi.mock('~/hooks/useMenuPosition', () => ({ useMenuPosition: () => ({ activeMenuId: null, menuPosition: null, handleMenuOpenChange: vi.fn() }) }));
vi.mock('./SecretActionsMenu', () => ({ default: () => null }));
vi.mock('wailsjs/go/main/App', () => ({
  DeleteSecret: vi.fn(), CancelListRequest: vi.fn(), GetSecretData: vi.fn(), GetSecretYaml: vi.fn(), ListAcceleratorSecretsMetadata: vi.fn(), ListSecretsMetadata: vi.fn(),
  SubscribeResourceWatcher: vi.fn(), SubscribeSecretWatcher: vi.fn(), UnsubscribeSecretWatcher: vi.fn(), UnsubscribeWatcher: vi.fn(),
}));
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(() => () => {}) }));

const source: SecretReadSource = {
  sourceKey: 'test-source', list: vi.fn(), cancelList: vi.fn(), subscribe: vi.fn(), unsubscribe: vi.fn(),
  getSecretData: vi.fn(), getSecretYaml: vi.fn(), onProgress: vi.fn(), onResource: vi.fn(), onStatus: vi.fn(), onError: vi.fn(), onConnected: vi.fn(),
} as any;

describe('SecretList', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockReturnValue({
      secrets: [
        { metadata: { uid: 'zero', name: 'zero', namespace: 'a', creationTimestamp: '2024-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 0 },
        { metadata: { uid: 'one', name: 'one', namespace: 'a', creationTimestamp: '2024-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 1 },
        { metadata: { uid: 'many', name: 'many', namespace: 'a', creationTimestamp: '2024-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 3 },
      ],
      loading: true,
      loadingProgress: { loaded: 2, total: 3 },
    });
  });

  it('preserves view behavior while rendering projected numeric keys without Size', () => {
    render(<SecretReadSourceProvider value={source}><SecretList isVisible={true} /></SecretReadSourceProvider>);
    expect(mocks.list).toHaveBeenCalledWith('ctx', ['a'], ['a', 'b'], true, true);
    const checkbox = screen.getByRole('checkbox', { name: /Hide Helm/i });
    expect((checkbox as HTMLInputElement).checked).toBe(true);
    fireEvent.click(checkbox);
    expect(mocks.list).toHaveBeenLastCalledWith('ctx', ['a'], ['a', 'b'], true, false);

    const props = mocks.resourceProps;
    expect(props.loadingProgress).toEqual({ loaded: 2, total: 3 });
    expect(props.getYamlApi).not.toBe(source.getSecretYaml);
    expect(props.data).toHaveLength(3);
    expect(props.columns.map((column: any) => column.key)).toEqual(['name', 'namespace', 'type', 'age', 'keys', 'actions']);
    expect(props.columns.map((column: any) => column.label)).not.toContain('Size');
    const keyColumn = props.columns.find((column: any) => column.key === 'keys');
    const { rerender } = render(<>{keyColumn.render(props.data[0])}</>);
    expect(screen.getByText('-')).toBeTruthy();
    rerender(<>{keyColumn.render(props.data[1])}</>);
    expect(screen.getByText('1 key')).toBeTruthy();
    rerender(<>{keyColumn.render(props.data[2])}</>);
    expect(screen.getByText('3 keys')).toBeTruthy();
    expect(keyColumn.getValue(props.data[2])).toBe(3);
    expect(props.selectable).toBe(true);
    expect(props.selection).toEqual({ selectedCount: 0 });
    props.onRowClick(props.data[1]);
    expect(mocks.editKeyValue).toHaveBeenCalledWith(props.data[1]);
    props.onNamespaceChange(['b']);
    expect(mocks.setSelectedNamespaces).toHaveBeenCalledWith(['b']);
    props.onBulkDelete([props.data[1]]);
    expect(mocks.openBulkDelete).toHaveBeenCalledWith([props.data[1]]);
  });
});
