import React, { useState } from 'react';
import { useAccelerator, useConfig, useK8s, useUI } from '~/context';
import { acceleratorPolicy, removeAcceleratorOverride, upsertAcceleratorOverride } from './acceleratorConfig';
import { acceleratorStateLabel } from './AcceleratorStatusBadge';
import type { AcceleratorDiagnostic } from '~/context/AcceleratorContext';
import Logger from '~/utils/Logger';

const busyStates = new Set(['sweeping', 'resolving', 'provisioning', 'connecting', 'reconnecting', 'draining', 'disposing']);
const failureDescriptions: Record<string, string> = {
  invalid_local_build: 'This desktop build does not have a publishable Accelerator release version.',
  descriptor_missing: 'No Accelerator release descriptor exists for this exact desktop build.',
  network_unavailable: 'The Accelerator release metadata could not be downloaded.',
  online_integrity: 'The downloaded Accelerator release metadata failed integrity validation.',
  cache_invalid: 'The cached Accelerator release metadata is invalid.',
  cache_io: 'The Accelerator release cache could not be read.',
  chart_pull_failed: 'The Accelerator Helm chart could not be pulled.',
  chart_integrity_failed: 'The Accelerator Helm chart failed integrity validation.',
  render_failed: 'The Accelerator Helm chart could not be rendered.',
  release_conflict: 'A conflicting Accelerator Helm release already exists.',
  permission_denied: 'The connection lacks permission to deploy the Accelerator resources.',
  install_failed: 'Helm could not install the Accelerator release.',
  job_failed: 'The Accelerator Kubernetes Job failed.',
  pod_failed: 'The Accelerator Pod failed.',
  image_pull_failed: 'Kubernetes could not pull the Accelerator image.',
  timeout: 'The Accelerator deployment timed out.',
  tunnel_unavailable: 'Kubikles could not open a tunnel to the Accelerator Pod.',
  accelerator_unavailable: 'The Accelerator process did not become available.',
  version_mismatch: 'The Accelerator and desktop build versions do not match exactly.',
};
const reasonText = (reason: string) => failureDescriptions[reason] ?? reason.replace(/_/g, ' ');

export default function AcceleratorConnectionPanel({ contextName, contextNamespace, onOpenSettings }: { contextName: string; contextNamespace: string; onOpenSettings: () => void }) {
  const { config, setConfig } = useConfig();
  const { currentContext, setSelectedNamespaces } = useK8s();
  const { navigateWithSearch } = useUI();
  const { status, enable, retry, disable } = useAccelerator();
  const [namespace, setNamespace] = useState(() => acceleratorPolicy(config.accelerator, contextName, contextNamespace || 'default').override?.namespace ?? '');
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const [actionError, setActionError] = useState('');
  const current = contextName === currentContext;
  const policy = acceleratorPolicy(config.accelerator, contextName, contextNamespace || 'default');
  const shown = current ? status : { state: 'direct_only', enabled: policy.enabled, namespace: policy.namespace, available: false };
  const save = (next: { enabled?: boolean; namespace?: string }) => {
    setConfig('accelerator.connectionOverrides', upsertAcceleratorOverride(config.accelerator, { contextName, ...policy.override, ...next }).connectionOverrides);
  };
  const deploy = async () => {
    save({ enabled: true, namespace });
    if (!current) return;
    if (shown.enabled && namespace !== (policy.override?.namespace ?? '') && !window.confirm('Changing the deployment namespace removes the current Accelerator workload before redeploying. Continue?')) return;
    await enable(namespace || policy.namespace);
  };
  const runAction = async (label: string, action: () => Promise<void>) => {
    setPendingAction(label);
    setActionError('');
    try {
      await action();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setActionError(message);
      Logger.error(`Accelerator ${label.toLowerCase()} request failed`, error, 'helm');
    } finally {
      setPendingAction(null);
    }
  };
  const remove = async () => {
    if (workload && !window.confirm('Disable Accelerator and remove its exact owned workload?')) return;
    save({ enabled: false });
    if (current) await disable();
  };
  const workload = (shown as any).workload;
  const diagnostics = ((shown as any).diagnostics ?? []) as AcceleratorDiagnostic[];
  const busy = pendingAction !== null || busyStates.has(shown.state);
  const view = (kind: 'helmreleases' | 'pods', name: string) => {
    setSelectedNamespaces([shown.namespace || policy.namespace]);
    navigateWithSearch(kind, name, true);
  };
  return <div className="space-y-4">
    <p className="text-xs text-gray-500">This connection owns Accelerator deployment. Secret screens only use an active session; they never start or remove one.</p>
    <div className="grid grid-cols-2 gap-2 text-xs">
      <div className="rounded border border-border bg-surface p-2"><span className="text-gray-500">Status</span><div className="mt-1 text-white">{acceleratorStateLabel(shown.state)}</div></div>
    </div>
    <p className="text-xs text-gray-500">Connection overrides inherit the global defaults. <button type="button" onClick={onOpenSettings} className="text-primary hover:underline">Open Settings &gt; Accelerator</button></p>
    <div>
      <label className="mb-1 block text-xs text-gray-400">Deployment namespace</label>
      <input value={namespace} onChange={event => setNamespace(event.target.value)} placeholder={contextNamespace || 'default'} className="w-full rounded border border-border bg-surface px-2.5 py-1.5 text-sm text-white focus:border-primary focus:outline-none" />
      <p className="mt-1 text-xs text-gray-500">Resolved: <span className="font-mono text-gray-300">{namespace || policy.namespace}</span> ({namespace === '' ? policy.namespaceSource : 'connection override'}). The namespace must already exist.</p>
    </div>
    <div className="flex flex-wrap gap-2">
      {!shown.enabled && <button disabled={busy} onClick={() => void runAction('Enable', deploy)} className="rounded bg-primary px-3 py-1.5 text-sm text-white disabled:cursor-wait disabled:opacity-50">{pendingAction === 'Enable' ? 'Starting...' : 'Enable & Deploy'}</button>}
      {shown.enabled && shown.state !== 'unavailable' && <button disabled={busy} onClick={() => void runAction('Redeploy', retry)} className="rounded bg-primary px-3 py-1.5 text-sm text-white disabled:cursor-wait disabled:opacity-50">{pendingAction === 'Redeploy' ? 'Redeploying...' : 'Redeploy'}</button>}
      {shown.state === 'unavailable' && shown.enabled && <button disabled={pendingAction !== null} onClick={() => void runAction('Retry', retry)} className="rounded bg-primary px-3 py-1.5 text-sm text-white disabled:cursor-wait disabled:opacity-50">{pendingAction === 'Retry' ? 'Retrying...' : 'Retry'}</button>}
      {shown.enabled && <button disabled={busy} onClick={() => void runAction('Removal', remove)} className="rounded border border-red-500/60 px-3 py-1.5 text-sm text-red-300 disabled:cursor-wait disabled:opacity-50">Disable & Remove</button>}
      <button onClick={() => setConfig('accelerator.connectionOverrides', removeAcceleratorOverride(config.accelerator, contextName).connectionOverrides)} className="rounded px-3 py-1.5 text-sm text-gray-400 hover:text-white">Reset overrides</button>
    </div>
    {actionError && <div role="alert" className="rounded border border-red-500/50 bg-red-950/30 p-3 text-xs text-red-200">Request failed: {actionError}</div>}
    {(shown.state === 'unavailable' || diagnostics.length > 0) && <div className="space-y-2 rounded border border-red-500/40 bg-red-950/20 p-3 text-xs">
      <div className="font-medium text-red-200">Deployment log</div>
      {diagnostics.length === 0 ? <div className="text-gray-400">No failure details were returned. Retry the deployment; request failures will appear here and in Debug logs.</div> : diagnostics.map((entry, index) => <div key={`${entry.timestamp}-${entry.phase}-${entry.reason}-${index}`} className="font-mono text-gray-300">
        <span className="text-gray-500">{entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : '--:--:--'}</span>{' '}
        <span className="text-red-300">{entry.phase}</span>: {reasonText(entry.reason)}{entry.attempt ? ` (attempt ${entry.attempt})` : ''}
      </div>)}
    </div>}
    {workload && <div className="space-y-2 border-t border-border pt-3 text-xs">
      <div className="text-gray-400">Release <span className="font-mono text-white">{workload.releaseName}</span>{workload.buildVersion && <> · build <span className="font-mono text-white">{workload.buildVersion}</span></>}</div>
      <div className="flex gap-2"><button onClick={() => view('helmreleases', workload.releaseName)} className="text-primary hover:underline">View Helm Release</button>{workload.pod?.name && <button onClick={() => view('pods', workload.pod.name)} className="text-primary hover:underline">View Pod</button>}</div>
    </div>}
  </div>;
}
