/** @vitest-environment jsdom */
import { act } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import * as entry from './entry';
import { createTestFacade } from './testFacade';

describe('Accelerator Browser entry', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/accelerator/browser/');
    document.body.innerHTML = '<div id="accelerator-browser-root"></div>';
  });
  afterEach(() => { document.head.querySelectorAll('[data-kubikles-accelerator-browser]').forEach(node => node.remove()); });

  it('mounts once into the exact root and cleans idempotently', async () => {
    const test = createTestFacade();
    let cleanup!: () => void;
    expect(Object.keys(entry)).toEqual(['mountAcceleratorBrowser']);
    expect(entry.mountAcceleratorBrowser).toEqual(expect.any(Function));
    act(() => { cleanup = entry.mountAcceleratorBrowser(test.facade); });
    await act(async () => {});
    expect(document.body.textContent).toContain('Kubikles Accelerator');
    expect(document.head.querySelector('link')?.getAttribute('href')).toBe('/accelerator/browser/assets/browser.css');
    let duplicateCleanup!: () => void;
    act(() => { duplicateCleanup = entry.mountAcceleratorBrowser(test.facade); });
    await act(async () => {});
    expect(document.body.textContent).toBe('Reopen from Kubikles');
    expect(duplicateCleanup).toBe(cleanup);
    act(() => { cleanup(); cleanup(); });
    expect(document.getElementById('accelerator-browser-root')?.childNodes).toHaveLength(0);
  });

  it('fails missing root synchronously and renders fixed terminal for invalid path or facade', async () => {
    document.body.innerHTML = '';
    expect(() => entry.mountAcceleratorBrowser(createTestFacade().facade)).toThrow('root is missing');
    expect(document.head.querySelector('[data-kubikles-accelerator-browser]')).toBeNull();
    document.body.innerHTML = '<div id="accelerator-browser-root"></div>';
    window.history.replaceState({}, '', '/wrong');
    let cleanup!: () => void;
    const valid = createTestFacade();
    act(() => { cleanup = entry.mountAcceleratorBrowser(valid.facade); });
    await act(async () => {});
    expect(document.body.textContent).toBe('Reopen from Kubikles');
    expect(window.location.pathname).toBe('/wrong');
    act(() => cleanup());
    window.history.replaceState({}, '', '/accelerator/browser/');
    const invalid = { ...valid.facade, bearer: 'HOSTILE' } as any;
    act(() => { cleanup = entry.mountAcceleratorBrowser(invalid); });
    await act(async () => {});
    expect(document.body.textContent).toBe('Reopen from Kubikles');
    expect(valid.facade.events.subscribe).not.toHaveBeenCalled();
    act(() => cleanup());
  });
});
