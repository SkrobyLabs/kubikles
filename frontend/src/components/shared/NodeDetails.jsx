import React, { useMemo, useState } from 'react';
import { PencilSquareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import { formatAge } from '../../utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import YamlEditor from './YamlEditor';
import NodeMetricsTab from './NodeMetricsTab';

const TAB_BASIC = 'basic';
const TAB_METRICS = 'metrics';

function formatBytes(bytes) {
    if (!bytes) return 'N/A';
    const num = parseInt(bytes.replace(/Ki$/, '')) * 1024;
    const gb = num / (1024 * 1024 * 1024);
    return `${gb.toFixed(1)} GB`;
}

function formatCPU(cpu) {
    if (!cpu) return 'N/A';
    if (cpu.endsWith('m')) {
        return `${(parseInt(cpu) / 1000).toFixed(2)} cores`;
    }
    return `${cpu} cores`;
}

export default function NodeDetails({ node, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch } = useUI();
    const [activeTab, setActiveTab] = useState(TAB_BASIC);

    const isStale = tabContext && tabContext !== currentContext;

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_METRICS, label: 'Metrics' },
    ], []);

    const name = node.metadata?.name;
    const labels = node.metadata?.labels || {};
    const annotations = node.metadata?.annotations || {};
    const spec = node.spec || {};
    const status = node.status || {};

    const conditions = status.conditions || [];
    const addresses = status.addresses || [];
    const nodeInfo = status.nodeInfo || {};
    const capacity = status.capacity || {};
    const allocatable = status.allocatable || {};
    const taints = spec.taints || [];

    const handleEditYaml = () => {
        const tabId = `yaml-node-${node.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${name}`,
            content: (
                <YamlEditor
                    resourceType="node"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleViewPods = () => {
        navigateWithSearch('pods', `node:"${name}"`);
    };

    const getConditionVariant = (condition) => {
        // For Ready, True is good
        // For others (MemoryPressure, DiskPressure, PIDPressure), False is good
        if (condition.type === 'Ready') {
            return condition.status === 'True' ? 'success' : 'error';
        }
        return condition.status === 'False' ? 'success' : 'error';
    };

    const isReady = conditions.find(c => c.type === 'Ready')?.status === 'True';

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {name}
                    </div>
                    <StatusBadge
                        status={isReady ? 'Ready' : 'NotReady'}
                        variant={isReady ? 'success' : 'error'}
                    />
                    {spec.unschedulable && (
                        <StatusBadge status="Cordoned" variant="warning" />
                    )}
                    {/* Tab Toggle */}
                    <div className="flex items-center bg-surface-light rounded-md p-0.5">
                        {tabs.map((tab) => (
                            <button
                                key={tab.id}
                                onClick={() => setActiveTab(tab.id)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    activeTab === tab.id
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                {tab.label}
                            </button>
                        ))}
                    </div>
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        <button
                            onClick={handleEditYaml}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Edit YAML"
                            disabled={isStale}
                        >
                            <PencilSquareIcon className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            {activeTab === TAB_METRICS ? (
                <NodeMetricsTab
                    nodeName={name}
                    isStale={isStale}
                />
            ) : (
            <div className="h-full overflow-auto p-4">
                {/* Capacity */}
                <DetailSection title="Capacity">
                    <div className="grid grid-cols-3 gap-4">
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-bold text-gray-200">{formatCPU(capacity.cpu)}</div>
                            <div className="text-xs text-gray-500">CPU Capacity</div>
                            <div className="text-xs text-gray-600 mt-1">{formatCPU(allocatable.cpu)} allocatable</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-bold text-gray-200">{formatBytes(capacity.memory)}</div>
                            <div className="text-xs text-gray-500">Memory Capacity</div>
                            <div className="text-xs text-gray-600 mt-1">{formatBytes(allocatable.memory)} allocatable</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-bold text-gray-200">{capacity.pods || 'N/A'}</div>
                            <div className="text-xs text-gray-500">Max Pods</div>
                            <div className="text-xs text-gray-600 mt-1">{allocatable.pods} allocatable</div>
                        </div>
                    </div>
                    <button
                        onClick={handleViewPods}
                        className="text-sm text-primary hover:text-primary/80 hover:underline mt-3"
                    >
                        View Pods on this Node →
                    </button>
                </DetailSection>

                {/* Conditions */}
                <DetailSection title="Conditions">
                    <div className="space-y-2">
                        {conditions.map((condition, idx) => (
                            <div key={idx} className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-0">
                                <div className="flex items-center gap-2">
                                    <StatusBadge status={condition.type} variant={getConditionVariant(condition)} />
                                    <span className="text-sm text-gray-400">{condition.message}</span>
                                </div>
                                <span className="text-xs text-gray-500" title={condition.lastHeartbeatTime}>
                                    {formatAge(condition.lastHeartbeatTime)}
                                </span>
                            </div>
                        ))}
                    </div>
                </DetailSection>

                {/* Addresses */}
                <DetailSection title="Addresses">
                    {addresses.map((addr, idx) => (
                        <DetailRow key={idx} label={addr.type}>
                            <CopyableLabel value={addr.address} />
                        </DetailRow>
                    ))}
                </DetailSection>

                {/* System Info */}
                <DetailSection title="System Info">
                    <DetailRow label="OS Image" value={nodeInfo.osImage} />
                    <DetailRow label="Architecture" value={nodeInfo.architecture} />
                    <DetailRow label="Kernel" value={nodeInfo.kernelVersion} />
                    <DetailRow label="Container Runtime" value={nodeInfo.containerRuntimeVersion} />
                    <DetailRow label="Kubelet" value={nodeInfo.kubeletVersion} />
                    <DetailRow label="Kube-Proxy" value={nodeInfo.kubeProxyVersion} />
                </DetailSection>

                {/* Taints */}
                {taints.length > 0 && (
                    <DetailSection title="Taints">
                        <div className="space-y-1.5">
                            {taints.map((taint, idx) => (
                                <div key={idx} className="flex items-center gap-2">
                                    <CopyableLabel
                                        value={`${taint.key}=${taint.value || ''}:${taint.effect}`}
                                    />
                                </div>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Metadata */}
                <DetailSection title="Metadata">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Created">
                        <span title={node.metadata?.creationTimestamp}>
                            {formatAge(node.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={node.metadata?.uid?.substring(0, 8) + '...'} copyValue={node.metadata?.uid} />
                    </DetailRow>
                </DetailSection>

                {/* Labels */}
                <DetailSection title="Labels">
                    <LabelsDisplay labels={labels} />
                </DetailSection>

                {/* Annotations */}
                <DetailSection title="Annotations">
                    <AnnotationsDisplay annotations={annotations} />
                </DetailSection>
            </div>
            )}
        </div>
    );
}
