/** @vitest-environment jsdom */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createRef } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { createBrowserSecretReadSource } from './browserSecretReadSource';
import BrowserSecretDetail from './BrowserSecretDetail';
import type { BrowserSecretDetailHandle } from './BrowserSecretDetail';
import { createTestFacade } from './testFacade';

const secret = { metadata: { name: 'one', namespace: 'team', uid: 'uid', creationTimestamp: '2026-01-01T00:00:00Z' }, type: 'Opaque', dataKeys: 2 };
const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(next => { resolve = next; });
  return { promise, resolve };
};

describe('Browser Secret detail', () => {
  it('loads concurrently and renders byte-safe read-only tabs only', async () => {
    const test = createTestFacade();
    (test.facade.GetSecretYaml as any).mockResolvedValue('kind: Secret\nmetadata:\n  name: one');
    (test.facade.GetSecretData as any).mockResolvedValue([
      { key: 'text', value: 'decoded', base64Value: 'ZGVjb2RlZA==', isBinary: false, source: 'data', encoding: 'text' },
      { key: 'binary', value: '', base64Value: '/wA=', isBinary: true, source: 'data', encoding: 'base64' },
    ]);
    const handle = createBrowserSecretReadSource(test.facade);
    handle.attach();
    render(<BrowserSecretDetail source={handle.source} secret={secret} onBack={() => {}} />);
    expect(test.facade.GetSecretYaml).toHaveBeenCalledWith('team', 'one');
    expect(test.facade.GetSecretData).toHaveBeenCalledWith('team', 'one');
    await screen.findByText('ZGVjb2RlZA==');
    fireEvent.click(screen.getByRole('button', { name: 'Decoded' }));
    expect(screen.getByText('decoded')).toBeTruthy();
    expect(screen.getByText('/wA=')).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Filter keys'), { target: { value: 'binary' } });
    expect(screen.queryByText('text')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'YAML' }));
    expect(screen.getByText(/kind: Secret/)).toBeTruthy();
    expect(document.body.innerHTML).not.toMatch(/textarea|contenteditable|Save|Delete|Download|Certificate|Copy/);
    handle.dispose();
  });

  it('clears rendered sensitive detail through its terminal fence', async () => {
    const test = createTestFacade();
    (test.facade.GetSecretYaml as any).mockResolvedValue('HOSTILE_YAML');
    (test.facade.GetSecretData as any).mockResolvedValue([{ key: 'HOSTILE_KEY', value: 'HOSTILE_VALUE', base64Value: 'SE9TVElMRQ==', isBinary: false, source: 'data', encoding: 'text' }]);
    const handle = createBrowserSecretReadSource(test.facade);
    handle.attach();
    const ref = createRef<BrowserSecretDetailHandle>();
    render(<BrowserSecretDetail ref={ref} source={handle.source} secret={secret} onBack={() => {}} />);
    await screen.findByText('SE9TVElMRQ==');
    act(() => ref.current?.clearSensitiveState());
    expect(document.body.innerHTML).not.toMatch(/HOSTILE|SE9TVElMRQ/);
    handle.dispose();
  });

  it('uses a fixed generic load error', async () => {
    const test = createTestFacade();
    (test.facade.GetSecretYaml as any).mockRejectedValue(new Error('HOSTILE_RAW_ERROR'));
    const handle = createBrowserSecretReadSource(test.facade);
    handle.attach();
    render(<BrowserSecretDetail source={handle.source} secret={secret} onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText('Unable to load Secret detail')).toBeTruthy());
    expect(document.body.innerHTML).not.toContain('HOSTILE_RAW_ERROR');
    handle.dispose();
  });

  it('fences deferred YAML and data across exact row replacement, Back, and unmount', async () => {
    const test = createTestFacade();
    const oldYaml = deferred<string>();
    const oldData = deferred<any[]>();
    const newYaml = deferred<string>();
    const newData = deferred<any[]>();
    (test.facade.GetSecretYaml as any).mockReturnValueOnce(oldYaml.promise).mockReturnValueOnce(newYaml.promise);
    (test.facade.GetSecretData as any).mockReturnValueOnce(oldData.promise).mockReturnValueOnce(newData.promise);
    const handle = createBrowserSecretReadSource(test.facade);
    handle.attach();
    const onBack = vi.fn();
    const next = { ...secret, metadata: { ...secret.metadata, name: 'two', uid: 'uid-two' } };
    const mounted = render(<BrowserSecretDetail source={handle.source} secret={secret} onBack={onBack} />);
    mounted.rerender(<BrowserSecretDetail source={handle.source} secret={next} onBack={onBack} />);
    newYaml.resolve('NEW_YAML');
    newData.resolve([{ key: 'NEW_KEY', value: 'new', base64Value: 'TkVX', isBinary: false, source: 'data', encoding: 'text' }]);
    await screen.findByText('TkVX');
    oldYaml.resolve('HOSTILE_OLD_YAML');
    oldData.resolve([{ key: 'HOSTILE_OLD_KEY', value: 'old', base64Value: 'T0xE', isBinary: false, source: 'data', encoding: 'text' }]);
    await act(async () => {});
    expect(document.body.innerHTML).not.toContain('HOSTILE_OLD');
    fireEvent.click(screen.getByRole('button', { name: 'Back' }));
    expect(onBack).toHaveBeenCalledOnce();
    expect(document.body.innerHTML).not.toContain('TkVX');
    mounted.unmount();
    handle.dispose();
  });
});
