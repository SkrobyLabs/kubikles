import React, { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { useConfig } from './ConfigContext';
import { useK8s } from './K8sContext';
import { DisableAccelerator, EnableAccelerator, GetAcceleratorStatus, RetryAccelerator } from '~/lib/wailsjs-adapter/go/main/App';
import { acceleratorPolicy } from '~/features/accelerator/acceleratorConfig';

type Status = { state: string; enabled: boolean; namespace: string; available: boolean };
type Value = { status: Status; enable: (namespaceOverride?: string) => Promise<void>; retry: () => Promise<void>; disable: () => Promise<void> };
const Context = createContext<Value | undefined>(undefined);
const direct: Status = { state: 'direct_only', enabled: false, namespace: '', available: false };

export function AcceleratorProvider({ children }: { children: React.ReactNode }) {
  const { config } = useConfig(); const { currentContext, currentNamespace } = useK8s();
  const [status, setStatus] = useState<Status>(direct);
  const sequence = useRef(0);
  const refresh = useCallback(async () => {
    if (!currentContext || typeof (window as any).go === 'undefined') { setStatus(direct); return; }
    const request = ++sequence.current;
    try { const next = await GetAcceleratorStatus(currentContext); if (request === sequence.current) setStatus(next); } catch { if (request === sequence.current) setStatus(direct); }
  }, [currentContext]);
  useEffect(() => { void refresh(); const id = window.setInterval(() => void refresh(), 2000); return () => clearInterval(id); }, [refresh]);
  const policy = acceleratorPolicy(config.accelerator, currentContext, currentNamespace || 'default');
  useEffect(() => {
    if (policy.enabled && currentContext && typeof (window as any).go !== 'undefined') {
      void EnableAccelerator(currentContext, policy.namespace).then(refresh).catch(() => setStatus(direct));
    }
  }, [currentContext, policy.enabled, policy.namespace, refresh]);
  const enable = useCallback(async (namespaceOverride?: string) => { await EnableAccelerator(currentContext, namespaceOverride ?? policy.namespace); await refresh(); }, [currentContext, policy.namespace, refresh]);
  const retry = useCallback(async () => { await RetryAccelerator(currentContext); await refresh(); }, [currentContext, refresh]);
  const disable = useCallback(async () => { await DisableAccelerator(currentContext); await refresh(); }, [currentContext, refresh]);
  return <Context.Provider value={{ status, enable, retry, disable }}>{children}</Context.Provider>;
}
export const useAccelerator = () => { const value = useContext(Context); if (!value) throw new Error('AcceleratorProvider missing'); return value; };
