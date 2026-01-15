import React from 'react';
import { PencilSquareIcon, ShareIcon, CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import { formatAge } from '../../utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import YamlEditor from './YamlEditor';
import DependencyGraph from './DependencyGraph';

export default function StorageClassDetails({ storageClass, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const isStale = tabContext && tabContext !== currentContext;

    const name = storageClass.metadata?.name;
    const labels = storageClass.metadata?.labels || {};
    const annotations = storageClass.metadata?.annotations || {};

    const provisioner = storageClass.provisioner;
    const reclaimPolicy = storageClass.reclaimPolicy || 'Delete';
    const volumeBindingMode = storageClass.volumeBindingMode || 'Immediate';
    const allowVolumeExpansion = storageClass.allowVolumeExpansion || false;
    const parameters = storageClass.parameters || {};
    const mountOptions = storageClass.mountOptions || [];

    // Check if this is the default storage class
    const isDefault = annotations['storageclass.kubernetes.io/is-default-class'] === 'true';

    const handleEditYaml = () => {
        const tabId = `yaml-storageclass-${storageClass.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${name}`,
            content: (
                <YamlEditor
                    resourceType="storageclass"
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-storageclass-${storageClass.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${name}`,
            content: (
                <DependencyGraph
                    resourceType="storageclass"
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const getReclaimPolicyVariant = () => {
        switch (reclaimPolicy) {
            case 'Delete': return 'error';
            case 'Retain': return 'success';
            case 'Recycle': return 'warning';
            default: return 'default';
        }
    };

    const getBindingModeVariant = () => {
        switch (volumeBindingMode) {
            case 'Immediate': return 'success';
            case 'WaitForFirstConsumer': return 'warning';
            default: return 'default';
        }
    };

    const parameterKeys = Object.keys(parameters);

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {name}
                    </div>
                    {isDefault && (
                        <span className="text-xs bg-blue-500/20 text-blue-400 px-1.5 py-0.5 rounded">default</span>
                    )}
                    <StatusBadge status={reclaimPolicy} variant={getReclaimPolicyVariant()} />
                    <StatusBadge status={volumeBindingMode} variant={getBindingModeVariant()} />
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
                {/* Summary */}
                <DetailSection title="Summary">
                    <div className="grid grid-cols-3 gap-4 mb-2">
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-bold text-gray-200">{reclaimPolicy}</div>
                            <div className="text-xs text-gray-500">Reclaim Policy</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-bold text-gray-200">{volumeBindingMode}</div>
                            <div className="text-xs text-gray-500">Binding Mode</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="flex items-center justify-center">
                                {allowVolumeExpansion ? (
                                    <CheckCircleIcon className="h-6 w-6 text-green-400" />
                                ) : (
                                    <XCircleIcon className="h-6 w-6 text-gray-500" />
                                )}
                            </div>
                            <div className="text-xs text-gray-500">Volume Expansion</div>
                        </div>
                    </div>
                </DetailSection>

                {/* Provisioner */}
                <DetailSection title="Provisioner">
                    <div className="p-3 bg-background-dark rounded border border-border">
                        <span className="font-mono text-sm text-gray-300">{provisioner}</span>
                    </div>
                </DetailSection>

                {/* Parameters */}
                <DetailSection title={`Parameters (${parameterKeys.length})`}>
                    {parameterKeys.length > 0 ? (
                        <div className="space-y-1.5">
                            {parameterKeys.map((key) => (
                                <div key={key} className="flex items-start gap-2 px-3 py-2 bg-background-dark rounded border border-border">
                                    <span className="font-mono text-sm text-gray-400 min-w-[150px]">{key}:</span>
                                    <span className="font-mono text-sm text-gray-300 break-all">{parameters[key]}</span>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <span className="text-gray-500">No parameters</span>
                    )}
                </DetailSection>

                {/* Mount Options */}
                {mountOptions.length > 0 && (
                    <DetailSection title={`Mount Options (${mountOptions.length})`}>
                        <div className="flex flex-wrap gap-1.5">
                            {mountOptions.map((option, idx) => (
                                <span
                                    key={idx}
                                    className="px-2 py-1 text-xs font-mono bg-gray-700/50 text-gray-300 rounded"
                                >
                                    {option}
                                </span>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Provisioner" value={provisioner} />
                    <DetailRow label="Reclaim Policy" value={reclaimPolicy} />
                    <DetailRow label="Volume Binding Mode" value={volumeBindingMode} />
                    <DetailRow label="Allow Volume Expansion" value={allowVolumeExpansion ? 'Yes' : 'No'} />
                    <DetailRow label="Default Class" value={isDefault ? 'Yes' : 'No'} />
                    <DetailRow label="Created">
                        <span title={storageClass.metadata?.creationTimestamp}>
                            {formatAge(storageClass.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={storageClass.metadata?.uid?.substring(0, 8) + '...'} copyValue={storageClass.metadata?.uid} />
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
