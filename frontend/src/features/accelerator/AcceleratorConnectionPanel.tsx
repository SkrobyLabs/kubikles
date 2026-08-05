import React, { useState } from 'react';
import { useAccelerator, useConfig, useK8s, useUI } from '~/context';
import { acceleratorPolicy, removeAcceleratorOverride, upsertAcceleratorOverride } from './acceleratorConfig';
import { acceleratorStateLabel } from './AcceleratorStatusBadge';

export default function AcceleratorConnectionPanel({ contextName, contextNamespace, onOpenSettings }: { contextName: string; contextNamespace: string; onOpenSettings: () => void }) {
  const { config, setConfig } = useConfig();
  const { currentContext, setSelectedNamespaces } = useK8s();
  const { navigateWithSearch } = useUI();
  const { status, enable, retry, disable } = useAccelerator();
  const [namespace, setNamespace] = useState(() => acceleratorPolicy(config.accelerator, contextName, contextNamespace || 'default').override?.namespace ?? '');
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
  const remove = async () => {
    if (!window.confirm('Disable Accelerator and remove its exact owned workload?')) return;
    save({ enabled: false });
    if (current) await disable();
  };
  const workload = (shown as any).workload;
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
      {!shown.enabled && <button onClick={() => void deploy()} className="rounded bg-primary px-3 py-1.5 text-sm text-white">Enable & Deploy</button>}
      {shown.enabled && <button onClick={() => void deploy()} className="rounded bg-primary px-3 py-1.5 text-sm text-white">Redeploy</button>}
      {shown.state === 'unavailable' && shown.enabled && <button onClick={() => void retry()} className="rounded bg-primary px-3 py-1.5 text-sm text-white">Retry</button>}
      {shown.enabled && <button onClick={() => void remove()} className="rounded border border-red-500/60 px-3 py-1.5 text-sm text-red-300">Disable & Remove</button>}
      <button onClick={() => setConfig('accelerator.connectionOverrides', removeAcceleratorOverride(config.accelerator, contextName).connectionOverrides)} className="rounded px-3 py-1.5 text-sm text-gray-400 hover:text-white">Reset overrides</button>
    </div>
    {workload && <div className="space-y-2 border-t border-border pt-3 text-xs">
      <div className="text-gray-400">Release <span className="font-mono text-white">{workload.releaseName}</span>{workload.buildVersion && <> · build <span className="font-mono text-white">{workload.buildVersion}</span></>}</div>
      <div className="flex gap-2"><button onClick={() => view('helmreleases', workload.releaseName)} className="text-primary hover:underline">View Helm Release</button>{workload.pod?.name && <button onClick={() => view('pods', workload.pod.name)} className="text-primary hover:underline">View Pod</button>}</div>
    </div>}
  </div>;
}
