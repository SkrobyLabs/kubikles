import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { flushSync } from 'react-dom';
import BrowserSecretDetail, { type BrowserSecretDetailHandle } from './BrowserSecretDetail';
import BrowserSecretList from './BrowserSecretList';
import { createBrowserSecretReadSource } from './browserSecretReadSource';
import type { AcceleratorBrowserFacade, AcceleratorSecretSummary } from './facade';

export function TerminalState() {
  return <main className="browser-terminal">Reopen from Kubikles</main>;
}

export default function BrowserApp({ facade }: { facade: AcceleratorBrowserFacade }) {
  const handle = useMemo(() => createBrowserSecretReadSource(facade), [facade]);
  const [ready, setReady] = useState(false);
  const [terminal, setTerminal] = useState(false);
  const [selected, setSelected] = useState<AcceleratorSecretSummary | null>(null);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [selectedNamespace, setSelectedNamespace] = useState('*');
  const [hideHelm, setHideHelm] = useState(true);
  const [search, setSearch] = useState('');
  const listStop = useRef<(() => Promise<void>) | null>(null);
  const detailRef = useRef<BrowserSecretDetailHandle>(null);
  const shutdown = useRef<Promise<void> | null>(null);
  const lifecycle = useRef(0);
  const layoutCommit = useRef(true);

  const registerStop = useCallback((stop: () => Promise<void>) => {
    listStop.current = stop;
    return () => {};
  }, []);
  const observeNamespaces = useCallback((observed: string[]) => {
    setNamespaces(previous => [...new Set([...previous, ...observed.filter(Boolean)])].sort());
  }, []);
  const selectSecret = useCallback((secret: AcceleratorSecretSummary) => {
    flushSync(() => setSelected(secret));
  }, []);
  const shutdownOnce = useCallback((stop: (() => Promise<void>) | null) => {
    if (!shutdown.current) shutdown.current = (async () => {
      if (stop) {
        try { await stop(); } catch { /* terminal cleanup remains fail closed */ }
      }
      handle.dispose();
      try { await facade.close(); } catch { /* the terminal UI is already final */ }
    })();
    return shutdown.current;
  }, [facade, handle]);

  useLayoutEffect(() => {
    const epoch = ++lifecycle.current;
    layoutCommit.current = true;
    const terminalDispose = handle.onTerminal(() => {
      const stop = listStop.current;
      const clear = () => {
        detailRef.current?.clearSensitiveState();
        setSelected(null);
        setNamespaces([]);
        setSelectedNamespace('*');
        setHideHelm(true);
        setSearch('');
        setTerminal(true);
      };
      if (layoutCommit.current) clear();
      else flushSync(clear);
      void shutdownOnce(stop);
    });
    const namespaceDispose = handle.source.onResource(event => observeNamespaces([event.resource?.metadata?.namespace]));
    if (handle.attach()) setReady(true);
    queueMicrotask(() => {
      if (lifecycle.current === epoch) layoutCommit.current = false;
    });
    return () => {
      const stop = listStop.current;
      handle.suspend();
      detailRef.current?.clearSensitiveState();
      queueMicrotask(() => {
        if (lifecycle.current !== epoch) {
          terminalDispose();
          namespaceDispose();
          return;
        }
        void shutdownOnce(stop);
      });
    };
  }, [handle, observeNamespaces, shutdownOnce]);

  if (terminal) return <TerminalState />;
  return <main className="browser-shell">
    <header><div className="browser-brand">Kubikles Accelerator</div><div>Context: <strong>in-cluster</strong></div></header>
    <h1>Secrets</h1>
    <div className="browser-toolbar">
      <label>Namespace<select aria-label="Namespace" value={selectedNamespace} onChange={event => setSelectedNamespace(event.target.value)}>
        <option value="*">All namespaces</option>{namespaces.map(namespace => <option key={namespace} value={namespace}>{namespace}</option>)}
      </select></label>
      <label className="browser-check"><input type="checkbox" checked={hideHelm} onChange={event => setHideHelm(event.target.checked)} /> Hide Helm</label>
      <label className="browser-search">Search Secrets<input aria-label="Search Secrets" value={search} onChange={event => setSearch(event.target.value)} /></label>
    </div>
    {ready && <>
      <BrowserSecretList visible={!selected} source={handle.source} selectedNamespace={selectedNamespace} hideHelm={hideHelm} search={search} onNamespacesObserved={observeNamespaces} onSelect={selectSecret} registerStop={registerStop} />
      {selected && <BrowserSecretDetail ref={detailRef} source={handle.source} secret={selected} onBack={() => setSelected(null)} />}
    </>}
  </main>;
}
