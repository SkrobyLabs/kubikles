import React from 'react';
import { PencilSquareIcon, ShareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import { formatAge } from '../../utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function PVDetails({ pv, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch } = useUI();

    const isStale = tabContext && tabContext !== currentContext;

    const name = pv.metadata?.name;
    const labels = pv.metadata?.labels || {};
    const annotations = pv.metadata?.annotations || {};
    const spec = pv.spec || {};
    const status = pv.status || {};

    const phase = status.phase || 'Unknown';
    const capacity = spec.capacity?.storage;
    const accessModes = spec.accessModes || [];
    const reclaimPolicy = spec.persistentVolumeReclaimPolicy;
    const storageClassName = spec.storageClassName;
    const volumeMode = spec.volumeMode || 'Filesystem';
    const claimRef = spec.claimRef;

    // Source type detection
    const getVolumeSource = () => {
        const sources = ['hostPath', 'nfs', 'csi', 'awsElasticBlockStore', 'gcePersistentDisk',
                         'azureDisk', 'azureFile', 'local', 'iscsi', 'fc', 'cephfs', 'rbd'];
        for (const src of sources) {
            if (spec[src]) {
                return { type: src, config: spec[src] };
            }
        }
        return null;
    };

    const volumeSource = getVolumeSource();

    const handleEditYaml = () => {
        const tabId = `yaml-pv-${pv.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${name}`,
            content: (
                <YamlEditor
                    resourceType="pv"
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-pv-${pv.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${name}`,
            content: (
                <DependencyGraph
                    resourceType="pv"
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleViewClaim = () => {
        if (claimRef) {
            navigateWithSearch('pvcs', `name:"${claimRef.name}" namespace:"${claimRef.namespace}"`);
        }
    };

    const getPhaseVariant = () => {
        switch (phase) {
            case 'Bound': return 'success';
            case 'Available': return 'primary';
            case 'Released': return 'warning';
            case 'Failed': return 'error';
            default: return 'default';
        }
    };

    const getReclaimPolicyVariant = () => {
        switch (reclaimPolicy) {
            case 'Delete': return 'error';
            case 'Retain': return 'success';
            case 'Recycle': return 'warning';
            default: return 'default';
        }
    };

    const getAccessModeColor = (mode) => {
        switch (mode) {
            case 'ReadWriteOnce': return 'bg-blue-500/20 text-blue-400';
            case 'ReadOnlyMany': return 'bg-yellow-500/20 text-yellow-400';
            case 'ReadWriteMany': return 'bg-green-500/20 text-green-400';
            case 'ReadWriteOncePod': return 'bg-purple-500/20 text-purple-400';
            default: return 'bg-gray-500/20 text-gray-400';
        }
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {name}
                    </div>
                    <StatusBadge status={phase} variant={getPhaseVariant()} />
                    {reclaimPolicy && (
                        <StatusBadge status={reclaimPolicy} variant={getReclaimPolicyVariant()} />
                    )}
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
                        <button
                            onClick={handleShowDependencies}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Dependencies"
                        >
                            <ShareIcon className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            <div className="h-full overflow-auto p-4">
                {/* Capacity */}
                <DetailSection title="Capacity">
                    <div className="text-center p-4 bg-background-dark rounded border border-border">
                        <div className="text-2xl font-bold text-gray-200">{capacity || '-'}</div>
                        <div className="text-xs text-gray-500">Storage</div>
                    </div>
                </DetailSection>

                {/* Access Modes */}
                <DetailSection title="Access Modes">
                    {accessModes.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {accessModes.map((mode, idx) => (
                                <span
                                    key={idx}
                                    className={`px-2 py-1 text-xs rounded ${getAccessModeColor(mode)}`}
                                >
                                    {mode}
                                </span>
                            ))}
                        </div>
                    ) : (
                        <span className="text-gray-500">None specified</span>
                    )}
                </DetailSection>

                {/* Claim Reference */}
                {claimRef && (
                    <DetailSection title="Claim">
                        <button
                            onClick={handleViewClaim}
                            className="text-primary hover:text-primary/80 hover:underline"
                        >
                            {claimRef.namespace}/{claimRef.name}
                        </button>
                    </DetailSection>
                )}

                {/* Volume Source */}
                {volumeSource && (
                    <DetailSection title="Volume Source">
                        <div className="p-3 bg-background-dark rounded border border-border">
                            <div className="text-sm font-medium text-gray-300 mb-2 capitalize">
                                {volumeSource.type}
                            </div>
                            <div className="space-y-1 text-xs">
                                {Object.entries(volumeSource.config).map(([key, value]) => (
                                    <div key={key} className="flex gap-2">
                                        <span className="text-gray-500">{key}:</span>
                                        <span className="text-gray-300 font-mono">
                                            {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                                        </span>
                                    </div>
                                ))}
                            </div>
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Storage Class" value={storageClassName || '-'} />
                    <DetailRow label="Volume Mode" value={volumeMode} />
                    <DetailRow label="Reclaim Policy" value={reclaimPolicy || '-'} />
                    <DetailRow label="Created">
                        <span title={pv.metadata?.creationTimestamp}>
                            {formatAge(pv.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={pv.metadata?.uid?.substring(0, 8) + '...'} copyValue={pv.metadata?.uid} />
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
        </div>
    );
}
