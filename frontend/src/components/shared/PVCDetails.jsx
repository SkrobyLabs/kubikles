import React, { useState, useEffect } from 'react';
import { PencilSquareIcon, ShareIcon, ArrowsPointingOutIcon, CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import { formatAge } from '../../utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import YamlEditor from './YamlEditor';
import DependencyGraph from './DependencyGraph';
import { GetStorageClass, ResizePVC } from '../../../wailsjs/go/main/App';

export default function PVCDetails({ pvc, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch } = useUI();

    const isStale = tabContext && tabContext !== currentContext;

    const name = pvc.metadata?.name;
    const namespace = pvc.metadata?.namespace;
    const labels = pvc.metadata?.labels || {};
    const annotations = pvc.metadata?.annotations || {};
    const spec = pvc.spec || {};
    const status = pvc.status || {};

    const phase = status.phase || 'Unknown';
    const volumeName = spec.volumeName;
    const storageClassName = spec.storageClassName;
    const accessModes = spec.accessModes || [];
    const volumeMode = spec.volumeMode || 'Filesystem';
    const requestedStorage = spec.resources?.requests?.storage;
    const actualCapacity = status.capacity?.storage;
    const conditions = status.conditions || [];

    // Resize functionality state
    const [allowVolumeExpansion, setAllowVolumeExpansion] = useState(null); // null = loading, true/false = result
    const [showResizeDialog, setShowResizeDialog] = useState(false);
    const [newSize, setNewSize] = useState('');
    const [resizeError, setResizeError] = useState(null);
    const [resizing, setResizing] = useState(false);

    // Fetch storage class to check if expansion is allowed
    useEffect(() => {
        if (storageClassName && !isStale) {
            GetStorageClass(storageClassName)
                .then((sc) => {
                    setAllowVolumeExpansion(sc?.allowVolumeExpansion || false);
                })
                .catch((err) => {
                    console.error('Failed to fetch storage class:', err);
                    setAllowVolumeExpansion(false);
                });
        } else {
            setAllowVolumeExpansion(false);
        }
    }, [storageClassName, isStale]);

    const canResize = allowVolumeExpansion && phase === 'Bound' && !isStale;

    const handleOpenResizeDialog = () => {
        if (!canResize) return;
        setNewSize(requestedStorage || '');
        setResizeError(null);
        setShowResizeDialog(true);
    };

    const handleResize = async () => {
        if (!newSize.trim()) {
            setResizeError('Please enter a new size');
            return;
        }
        setResizing(true);
        setResizeError(null);
        try {
            await ResizePVC(namespace, name, newSize.trim());
            setShowResizeDialog(false);
        } catch (err) {
            setResizeError(err.message || String(err));
        } finally {
            setResizing(false);
        }
    };

    const handleEditYaml = () => {
        const tabId = `yaml-pvc-${pvc.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${name}`,
            content: (
                <YamlEditor
                    resourceType="pvc"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-pvc-${pvc.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${name}`,
            content: (
                <DependencyGraph
                    resourceType="pvc"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleViewPV = () => {
        if (volumeName) {
            navigateWithSearch('pvs', `name:"${volumeName}"`);
        }
    };

    const getPhaseVariant = () => {
        switch (phase) {
            case 'Bound': return 'success';
            case 'Pending': return 'warning';
            case 'Lost': return 'error';
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
        <div className="flex flex-col h-full bg-[#1e1e1e]">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {namespace}/{name}
                    </div>
                    <StatusBadge status={phase} variant={getPhaseVariant()} />
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

            {/* Resize Dialog */}
            {showResizeDialog && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
                    <div className="bg-gray-800 rounded-lg shadow-xl w-full max-w-md mx-4 overflow-hidden">
                        <div className="px-4 py-3 border-b border-gray-700">
                            <h3 className="text-lg font-medium text-white">Resize PVC</h3>
                        </div>
                        <div className="p-4 space-y-4">
                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-1">
                                    Current Size
                                </label>
                                <div className="text-sm text-gray-400 bg-gray-700 px-3 py-2 rounded">
                                    {requestedStorage || '-'}
                                </div>
                            </div>
                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-1">
                                    New Size
                                </label>
                                <input
                                    type="text"
                                    value={newSize}
                                    onChange={(e) => setNewSize(e.target.value)}
                                    placeholder="e.g., 20Gi, 100Gi"
                                    className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white placeholder-gray-500 focus:outline-none focus:border-blue-500"
                                />
                                <p className="mt-1 text-xs text-gray-500">
                                    New size must be larger than current. Use Kubernetes size notation (e.g., 10Gi, 500Mi)
                                </p>
                            </div>
                            {resizeError && (
                                <div className="text-sm text-red-400 bg-red-400/10 px-3 py-2 rounded">
                                    {resizeError}
                                </div>
                            )}
                        </div>
                        <div className="flex justify-end gap-2 px-4 py-3 border-t border-gray-700 bg-gray-800/50">
                            <button
                                onClick={() => setShowResizeDialog(false)}
                                className="px-3 py-1.5 text-sm text-gray-300 hover:text-white hover:bg-gray-700 rounded transition-colors"
                                disabled={resizing}
                            >
                                Cancel
                            </button>
                            <button
                                onClick={handleResize}
                                disabled={resizing}
                                className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded transition-colors"
                            >
                                {resizing ? (
                                    <div className="h-4 w-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                                ) : (
                                    <ArrowsPointingOutIcon className="h-4 w-4" />
                                )}
                                Resize
                            </button>
                        </div>
                    </div>
                </div>
            )}

            {/* Content Area */}
            <div className="h-full overflow-auto p-4">
                {/* Capacity */}
                <DetailSection title="Capacity">
                    <div className="grid grid-cols-2 gap-4">
                        <div
                            className={`text-center p-3 bg-[#1a1a1a] rounded border ${
                                canResize
                                    ? 'border-green-500/50 cursor-pointer hover:bg-green-500/10 transition-colors'
                                    : 'border-border'
                            }`}
                            onClick={handleOpenResizeDialog}
                            title={canResize ? 'Click to resize' : allowVolumeExpansion === false ? 'Resize not supported by storage class' : ''}
                        >
                            <div className="text-lg font-bold text-gray-200">{requestedStorage || '-'}</div>
                            <div className="text-xs text-gray-500 flex items-center justify-center gap-1">
                                Requested
                                {allowVolumeExpansion !== null && (
                                    canResize ? (
                                        <CheckCircleIcon className="h-3.5 w-3.5 text-green-400" title="Resize supported" />
                                    ) : (
                                        <XCircleIcon className="h-3.5 w-3.5 text-red-400" title="Resize not supported" />
                                    )
                                )}
                            </div>
                        </div>
                        <div className="text-center p-3 bg-[#1a1a1a] rounded border border-border">
                            <div className="text-lg font-bold text-green-400">{actualCapacity || '-'}</div>
                            <div className="text-xs text-gray-500">Actual</div>
                        </div>
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

                {/* Bound Volume */}
                {volumeName && (
                    <DetailSection title="Bound Volume">
                        <button
                            onClick={handleViewPV}
                            className="text-primary hover:text-primary/80 hover:underline"
                        >
                            {volumeName}
                        </button>
                    </DetailSection>
                )}

                {/* Conditions */}
                {conditions.length > 0 && (
                    <DetailSection title="Conditions">
                        <div className="space-y-2">
                            {conditions.map((condition, idx) => (
                                <div key={idx} className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-0">
                                    <div className="flex items-center gap-2">
                                        <StatusBadge
                                            status={condition.type}
                                            variant={condition.status === 'True' ? 'success' : 'default'}
                                        />
                                        <span className="text-sm text-gray-400">{condition.message}</span>
                                    </div>
                                    <span className="text-xs text-gray-500">
                                        {formatAge(condition.lastTransitionTime)}
                                    </span>
                                </div>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Storage Class" value={storageClassName || '-'} />
                    <DetailRow label="Volume Mode" value={volumeMode} />
                    <DetailRow label="Created">
                        <span title={pvc.metadata?.creationTimestamp}>
                            {formatAge(pvc.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={pvc.metadata?.uid?.substring(0, 8) + '...'} copyValue={pvc.metadata?.uid} />
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
