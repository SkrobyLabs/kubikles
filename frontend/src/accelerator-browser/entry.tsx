import { createRoot, type Root } from 'react-dom/client';
import BrowserApp, { TerminalState } from './BrowserApp';
import { isAcceleratorBrowserFacade, type AcceleratorBrowserFacade } from './facade';
import './browser.css';

const mounts = new WeakMap<Element, { root: Root; cleanup: () => void }>();

export function mountAcceleratorBrowser(facade: AcceleratorBrowserFacade): () => void {
  const element = document.getElementById('accelerator-browser-root');
  if (!element) throw new Error('Accelerator Browser root is missing');

  const existing = mounts.get(element);
  if (existing) {
    existing.root.render(<TerminalState />);
    return existing.cleanup;
  }

  const root = createRoot(element);
  let cleaned = false;
  let style: HTMLLinkElement | null = null;
  const cleanup = () => {
    if (cleaned) return;
    cleaned = true;
    mounts.delete(element);
    root.unmount();
    style?.remove();
    element.replaceChildren();
  };
  mounts.set(element, { root, cleanup });

  if (!isAcceleratorBrowserFacade(facade) || window.location.pathname !== '/accelerator/browser/') {
    root.render(<TerminalState />);
    return cleanup;
  }

  style = document.createElement('link');
  style.rel = 'stylesheet';
  style.href = '/accelerator/browser/assets/browser.css';
  style.dataset.kubiklesAcceleratorBrowser = 'v1';
  document.head.append(style);
  root.render(<BrowserApp facade={facade} />);
  return cleanup;
}
