import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { useConfig } from './ConfigContext';
import { useK8s } from './K8sContext';
import { DisableAccelerator, EnableAccelerator, GetAcceleratorStatus, RetryAccelerator } from '~/lib/wailsjs-adapter/go/main/App';
import { acceleratorPolicy } from '~/features/accelerator/acceleratorConfig';
import Logger from '~/utils/Logger';

export type AcceleratorDiagnostic = { timestamp: string; phase: string; reason: string; attempt?: number };
type Status = { state: string; enabled: boolean; namespace: string; available: boolean; versionMismatchWarning?: boolean; diagnostics?: AcceleratorDiagnostic[]; workload?: unknown };
type Value = { status: Status; enable: (namespaceOverride?: string) => Promise<void>; retry: () => Promise<void>; disable: () => Promise<void> };
const Context = createContext<Value | undefined>(undefined);
const direct: Status = { state: 'direct_only', enabled: false, namespace: '', available: false };

export function AcceleratorProvider({ children }: { children: React.ReactNode }) {
  const { config } = useConfig(); const { currentContext } = useK8s();
  const [status, setStatus] = useState<Status>(direct);
  const sequence = useRef(0);
  const loggedVersionWarning = useRef('');
  const refresh = useCallback(async () => {
    if (!currentContext || typeof (window as any).go === 'undefined') { setStatus(direct); return; }
    const request = ++sequence.current;
    try { const next = await GetAcceleratorStatus(currentContext); if (request === sequence.current) setStatus(next); } catch (error) { if (request === sequence.current) setStatus(direct); Logger.error('Failed to read Accelerator deployment status', error, 'helm'); }
  }, [currentContext]);
  useEffect(() => {
    if (!status.versionMismatchWarning || loggedVersionWarning.current === currentContext) return;
    loggedVersionWarning.current = currentContext;
    Logger.warn('Accelerator connected with a version mismatch from custom artifacts', { context: currentContext }, 'helm');
  }, [currentContext, status.versionMismatchWarning]);
  useEffect(() => { void refresh(); const id = window.setInterval(() => void refresh(), 2000); return () => clearInterval(id); }, [refresh]);
  // An empty namespace delegates to the namespace declared by the kubeconfig
  // context. UI namespace selection is view state and must never retarget the
  // Accelerator workload when the user navigates between views.
  const policy = acceleratorPolicy(config.accelerator, currentContext, '');
  const deploymentOptions = useMemo(() => JSON.stringify({
    imageReference: config.accelerator.customImage?.trim() ?? '',
    chartReference: config.accelerator.customChart?.trim() ?? '',
  }), [config.accelerator.customChart, config.accelerator.customImage]);
  useEffect(() => {
    if (policy.enabled && currentContext && typeof (window as any).go !== 'undefined') {
      void EnableAccelerator(currentContext, policy.namespace, deploymentOptions).then(refresh).catch((error: unknown) => { setStatus(direct); Logger.error('Failed to enable Accelerator', error, 'helm'); });
    }
  }, [currentContext, deploymentOptions, policy.enabled, policy.namespace, refresh]);
  const enable = useCallback(async (namespaceOverride?: string) => { Logger.info('Enabling Accelerator deployment', { context: currentContext, namespace: namespaceOverride ?? policy.namespace }, 'helm'); await EnableAccelerator(currentContext, namespaceOverride ?? policy.namespace, deploymentOptions); await refresh(); }, [currentContext, deploymentOptions, policy.namespace, refresh]);
  const retry = useCallback(async () => { Logger.info('Retrying Accelerator deployment', { context: currentContext }, 'helm'); await RetryAccelerator(currentContext); await refresh(); }, [currentContext, refresh]);
  const disable = useCallback(async () => { Logger.info('Removing Accelerator deployment', { context: currentContext }, 'helm'); await DisableAccelerator(currentContext); await refresh(); }, [currentContext, refresh]);
  return <Context.Provider value={{ status, enable, retry, disable }}>{children}</Context.Provider>;
}
export const useAccelerator = () => { const value = useContext(Context); if (!value) throw new Error('AcceleratorProvider missing'); return value; };
