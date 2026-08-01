/** @vitest-environment jsdom */
import React from 'react';
import { render, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ResourceList from './ResourceList';

const runtime = vi.hoisted(() => ({ EventsOn: vi.fn(() => vi.fn()) }));
vi.mock('wailsjs/runtime/runtime', () => runtime);
vi.mock('react-virtuoso', () => ({ TableVirtuoso: () => <div data-testid="table" /> }));
vi.mock('~/context', () => ({
  useUI: () => ({ pendingSearch: null, consumePendingSearch: vi.fn() }),
  useConfig: () => ({ getConfig: (key: string) => key === 'ui.largeDatasetThreshold' ? 5000 : 0 }),
  useK8s: () => ({ refreshNamespaces: vi.fn() }),
}));
vi.mock('~/hooks/useSavedViews', () => ({
  useSavedViews: () => ({
    views: [], saveView: vi.fn(), loadView: vi.fn(), updateView: vi.fn(), deleteView: vi.fn(), renameView: vi.fn(),
    duplicateView: vi.fn(), setDefaultView: vi.fn(), getDefaultView: vi.fn(() => null),
  }),
}));

const baseProps = {
  title: 'Secrets',
  columns: [{ key: 'name', label: 'Name', render: (item: any) => item.metadata.name }],
  data: [],
  isLoading: true,
  resourceType: 'secrets',
};

describe('ResourceList controlled progress', () => {
  beforeEach(() => runtime.EventsOn.mockClear());

  it('does not register or fall back to global progress for explicit null or object', async () => {
    const { rerender } = render(<ResourceList {...baseProps} loadingProgress={null} />);
    await waitFor(() => expect(runtime.EventsOn).not.toHaveBeenCalled());
    rerender(<ResourceList {...baseProps} loadingProgress={{ loaded: 2, total: 8 }} />);
    await waitFor(() => expect(runtime.EventsOn).not.toHaveBeenCalled());
  });

  it('preserves the legacy listener when progress is omitted', async () => {
    const { unmount } = render(<ResourceList {...baseProps} />);
    await waitFor(() => expect(runtime.EventsOn).toHaveBeenCalledWith('list-progress', expect.any(Function)));
    const dispose = runtime.EventsOn.mock.results[0].value;
    unmount();
    expect(dispose).toHaveBeenCalledOnce();
  });
});
