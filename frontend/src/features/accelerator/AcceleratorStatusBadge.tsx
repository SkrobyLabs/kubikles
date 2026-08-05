import React from 'react';
import { useAccelerator } from '~/context';

const labels: Record<string, string> = {
  direct_only: 'Disabled', sweeping: 'Deploying', resolving: 'Deploying', provisioning: 'Deploying',
  connecting: 'Connecting', active: 'Active', reconnecting: 'Retrying', draining: 'Removing',
  disposing: 'Removing', unavailable: 'Unavailable', closed: 'Unavailable',
};

export function acceleratorStateLabel(state: string) { return labels[state] || 'Unavailable'; }

export default function AcceleratorStatusBadge({ onOpen }: { onOpen?: () => void }) {
  const { status } = useAccelerator();
  const label = acceleratorStateLabel(status.state);
  const color = status.available ? 'bg-green-500' : status.enabled ? 'bg-yellow-500' : 'bg-gray-500';
  return (
    <button onClick={onOpen} className="mt-2 flex w-full items-center gap-2 rounded px-1 py-1 text-left text-xs text-gray-400 hover:bg-white/5 hover:text-white" title="Open Accelerator settings">
      <span className={`h-2 w-2 shrink-0 rounded-full ${color}`} />
      <span>Accelerator: {label}</span>
      {status.namespace && <span className="ml-auto truncate font-mono text-gray-500">{status.namespace}</span>}
    </button>
  );
}
