import React, { useMemo } from 'react';
import { PencilSquareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, StatusBadge, CopyableLabel } from './DetailComponents';
import { entriesFromObject, matchesSearch, normalizeSearchTerm, NoSectionMatches, useSectionSearch } from './detailSearch';
import { LazyYamlEditor as YamlEditor } from '../lazy';
import NodeMetricsTab from './NodeMetricsTab';

const TAB_BASIC = 'basic';
const TAB_METRICS = 'metrics';

function formatBytes(bytes: any) {
    if (!bytes) return 'N/A';
    const num = parseInt(bytes.replace(/Ki$/, '')) * 1024;
    const gb = num / (1024 * 1024 * 1024);
    return `${gb.toFixed(1)} GB`;
}

function formatCPU(cpu: any) {
    if (!cpu) return 'N/A';
    if (cpu.endsWith('m')) {
        return `${(parseInt(cpu) / 1000).toFixed(2)} cores`;
    }
    return `${cpu} cores`;
}

export default function NodeDetails({ node, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch, getDetailTab, setDetailTab } = useUI();
    const activeTab = getDetailTab('node', TAB_BASIC);
    const setActiveTab = (tab: any) => setDetailTab('node', tab);

    const isStale = tabContext && tabContext !== currentContext;
    const resourceContext = tabContext || currentContext;

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
    const { sectionSearch, getSectionTerm, renderSearch } = useSectionSearch();

    const systemInfoRows = useMemo(() => [
        { label: 'OS Image', value: nodeInfo.osImage },
        { label: 'Architecture', value: nodeInfo.architecture },
        { label: 'Kernel', value: nodeInfo.kernelVersion },
        { label: 'Container Runtime', value: nodeInfo.containerRuntimeVersion },
        { label: 'Kubelet', value: nodeInfo.kubeletVersion },
        { label: 'Kube-Proxy', value: nodeInfo.kubeProxyVersion },
    ], [nodeInfo]);

    const metadataRows = useMemo(() => [
        { label: 'Name', value: name, copyValue: name },
        {
            label: 'Created',
            value: node.metadata?.creationTimestamp ? `${formatAge(node.metadata.creationTimestamp)} ago` : 'N/A',
            title: node.metadata?.creationTimestamp
        },
        {
            label: 'UID',
            value: node.metadata?.uid ? `${node.metadata.uid.substring(0, 8)}...` : 'N/A',
            copyValue: node.metadata?.uid
        },
    ], [name, node.metadata?.creationTimestamp, node.metadata?.uid]);

    const labelEntries = useMemo(() => entriesFromObject(labels), [labels]);
    const annotationEntries = useMemo(() => entriesFromObject(annotations), [annotations]);

    const filteredConditions = useMemo(() => (
        conditions.filter((condition: any) => matchesSearch([
            condition.type,
            condition.status,
            condition.reason,
            condition.message,
            condition.lastHeartbeatTime,
            condition.lastTransitionTime,
        ], getSectionTerm('conditions')))
    ), [conditions, sectionSearch]);

    const filteredAddresses = useMemo(() => (
        addresses.filter((addr: any) => matchesSearch([
            addr.type,
            addr.address,
        ], getSectionTerm('addresses')))
    ), [addresses, sectionSearch]);

    const filteredSystemInfoRows = useMemo(() => (
        systemInfoRows.filter((row) => matchesSearch([
            row.label,
            row.value,
        ], getSectionTerm('systemInfo')))
    ), [systemInfoRows, sectionSearch]);

    const filteredTaints = useMemo(() => (
        taints.filter((taint: any) => matchesSearch([
            taint.key,
            taint.value,
            taint.effect,
            `${taint.key}=${taint.value || ''}:${taint.effect}`,
        ], getSectionTerm('taints')))
    ), [taints, sectionSearch]);

    const filteredMetadataRows = useMemo(() => (
        metadataRows.filter((row) => matchesSearch([
            row.label,
            row.value,
            row.copyValue,
            row.title,
        ], getSectionTerm('metadata')))
    ), [metadataRows, sectionSearch]);

    const filteredLabels = useMemo(() => (
        labelEntries.filter((entry) => matchesSearch([
            entry.key,
            entry.value,
            entry.display,
        ], getSectionTerm('labels')))
    ), [labelEntries, sectionSearch]);

    const filteredAnnotations = useMemo(() => (
        annotationEntries.filter((entry) => matchesSearch([
            entry.key,
            entry.value,
            entry.display,
        ], getSectionTerm('annotations')))
    ), [annotationEntries, sectionSearch]);

    const handleEditYaml = () => {
        const tabId = `${resourceContext}-yaml-node-${name}`;
        openTab({
            id: tabId,
            context: resourceContext,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="node"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={resourceContext}
                />
            )
        });
    };

    const handleViewPods = () => {
        navigateWithSearch('pods', `node:"${name}"`);
    };

    const getConditionVariant = (condition: any) => {
        // For Ready, True is good
        // For others (MemoryPressure, DiskPressure, PIDPressure), False is good
        if (condition.type === 'Ready') {
            return condition.status === 'True' ? 'success' : 'error';
        }
        return condition.status === 'False' ? 'success' : 'error';
    };

    const isReady = conditions.find((c: any) => c.type === 'Ready')?.status === 'True';

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
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
                <DetailSection title="Conditions" headerAction={renderSearch('conditions', 'Search conditions...')}>
                    <div className="space-y-2">
                        {filteredConditions.map((condition: any, idx: number) => (
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
                        {conditions.length > 0 && normalizeSearchTerm(getSectionTerm('conditions')) && filteredConditions.length === 0 && (
                            <NoSectionMatches term={getSectionTerm('conditions')} />
                        )}
                    </div>
                </DetailSection>

                {/* Addresses */}
                <DetailSection title="Addresses" headerAction={renderSearch('addresses', 'Search addresses...')}>
                    {filteredAddresses.map((addr: any, idx: number) => (
                        <DetailRow key={idx} label={addr.type}>
                            <CopyableLabel value={addr.address} />
                        </DetailRow>
                    ))}
                    {addresses.length > 0 && normalizeSearchTerm(getSectionTerm('addresses')) && filteredAddresses.length === 0 && (
                        <NoSectionMatches term={getSectionTerm('addresses')} />
                    )}
                </DetailSection>

                {/* System Info */}
                <DetailSection title="System Info" headerAction={renderSearch('systemInfo', 'Search system info...')}>
                    {filteredSystemInfoRows.map((row) => (
                        <DetailRow key={row.label} label={row.label} value={row.value} />
                    ))}
                    {systemInfoRows.length > 0 && normalizeSearchTerm(getSectionTerm('systemInfo')) && filteredSystemInfoRows.length === 0 && (
                        <NoSectionMatches term={getSectionTerm('systemInfo')} />
                    )}
                </DetailSection>

                {/* Taints */}
                {taints.length > 0 && (
                    <DetailSection title="Taints" headerAction={renderSearch('taints', 'Search taints...')}>
                        <div className="space-y-1.5">
                            {filteredTaints.map((taint: any, idx: number) => (
                                <div key={idx} className="flex items-center gap-2">
                                    <CopyableLabel
                                        value={`${taint.key}=${taint.value || ''}:${taint.effect}`}
                                    />
                                </div>
                            ))}
                            {normalizeSearchTerm(getSectionTerm('taints')) && filteredTaints.length === 0 && (
                                <NoSectionMatches term={getSectionTerm('taints')} />
                            )}
                        </div>
                    </DetailSection>
                )}

                {/* Metadata */}
                <DetailSection title="Metadata" headerAction={renderSearch('metadata', 'Search metadata...')}>
                    {filteredMetadataRows.map((row) => (
                        <DetailRow key={row.label} label={row.label}>
                            {row.label === 'UID' ? (
                                <CopyableLabel value={row.value} copyValue={row.copyValue} />
                            ) : row.title ? (
                                <span title={row.title}>{row.value}</span>
                            ) : (
                                row.value
                            )}
                        </DetailRow>
                    ))}
                    {metadataRows.length > 0 && normalizeSearchTerm(getSectionTerm('metadata')) && filteredMetadataRows.length === 0 && (
                        <NoSectionMatches term={getSectionTerm('metadata')} />
                    )}
                </DetailSection>

                {/* Labels */}
                <DetailSection title="Labels" headerAction={renderSearch('labels', 'Search labels...')}>
                    {labelEntries.length === 0 ? (
                        <span className="text-gray-500">None</span>
                    ) : filteredLabels.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {filteredLabels.map((entry) => (
                                <CopyableLabel key={entry.key} value={entry.display} />
                            ))}
                        </div>
                    ) : (
                        <NoSectionMatches term={getSectionTerm('labels')} />
                    )}
                </DetailSection>

                {/* Annotations */}
                <DetailSection title="Annotations" headerAction={renderSearch('annotations', 'Search annotations...')}>
                    {annotationEntries.length === 0 ? (
                        <span className="text-gray-500">None</span>
                    ) : filteredAnnotations.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {filteredAnnotations.map((entry) => (
                                <CopyableLabel
                                    key={entry.key}
                                    value={entry.display}
                                    copyValue={entry.display}
                                    className="bg-purple-500/10 border-purple-500/30"
                                />
                            ))}
                        </div>
                    ) : (
                        <NoSectionMatches term={getSectionTerm('annotations')} />
                    )}
                </DetailSection>
            </div>
            )}
        </div>
    );
}
