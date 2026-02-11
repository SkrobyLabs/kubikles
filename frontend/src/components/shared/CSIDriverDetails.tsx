import React from 'react';
import { PencilSquareIcon, ShareIcon, CheckCircleIcon, XCircleIcon, CpuChipIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

const BooleanBadge = ({ value, label }: any) => {
    return (
        <div className="flex items-center gap-2">
            {value ? (
                <CheckCircleIcon className="h-4 w-4 text-green-400" />
            ) : (
                <XCircleIcon className="h-4 w-4 text-gray-500" />
            )}
            <span className={value ? 'text-green-400' : 'text-gray-500'}>
                {value ? 'Yes' : 'No'}
            </span>
        </div>
    );
};

export default function CSIDriverDetails({ csiDriver, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = csiDriver?.metadata || {};
    const spec = csiDriver?.spec || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-csidriver-${csiDriver.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: CpuChipIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="csidriver"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-csidriver-${csiDriver.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: CpuChipIcon,
            content: (
                <DependencyGraph
                    resourceType="csidriver"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const capabilities = [
        { label: 'Attach Required', value: spec.attachRequired ?? true },
        { label: 'Pod Info on Mount', value: spec.podInfoOnMount ?? false },
        { label: 'Storage Capacity', value: spec.storageCapacity ?? false },
        { label: 'SELinux Mount', value: spec.seLinuxMount ?? false },
    ];

    const volumeLifecycleModes = spec.volumeLifecycleModes || [];
    const fsGroupPolicy = spec.fsGroupPolicy;
    const tokenRequests = spec.tokenRequests || [];

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {name}
                    </div>
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        <button
                            onClick={handleEditYaml}
                            className={`p-1.5 rounded transition-colors ${isStale ? 'text-gray-600 cursor-not-allowed' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
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
                {/* Capabilities */}
                <DetailSection title="Capabilities">
                    <div className="grid grid-cols-2 gap-4">
                        {capabilities.map(({ label, value }) => (
                            <div key={label}>
                                <dt className="text-xs text-gray-500">{label}</dt>
                                <dd className="text-sm mt-0.5">
                                    <BooleanBadge value={value} />
                                </dd>
                            </div>
                        ))}
                    </div>
                </DetailSection>

                {/* Volume Lifecycle Modes */}
                <DetailSection title="Volume Lifecycle Modes">
                    {volumeLifecycleModes.length === 0 ? (
                        <span className="text-gray-500">No modes specified (defaults to Persistent)</span>
                    ) : (
                        <div className="flex flex-wrap gap-2">
                            {volumeLifecycleModes.map((mode: any) => (
                                <span
                                    key={mode}
                                    className="inline-flex items-center px-2.5 py-1 rounded text-xs bg-blue-500/20 text-blue-400"
                                >
                                    {mode}
                                </span>
                            ))}
                        </div>
                    )}
                </DetailSection>

                {/* FS Group Policy */}
                {fsGroupPolicy && (
                    <DetailSection title="FS Group Policy">
                        <div className="bg-background-dark rounded border border-border p-3">
                            <span className={`text-sm ${
                                fsGroupPolicy === 'ReadWriteOnceWithFSType' ? 'text-green-400' :
                                fsGroupPolicy === 'File' ? 'text-blue-400' :
                                fsGroupPolicy === 'None' ? 'text-gray-400' : 'text-gray-300'
                            }`}>
                                {fsGroupPolicy}
                            </span>
                            <p className="text-xs text-gray-500 mt-1">
                                {fsGroupPolicy === 'ReadWriteOnceWithFSType' && 'CSI driver supports fsGroup if volumeMode is Filesystem and accessMode is ReadWriteOnce'}
                                {fsGroupPolicy === 'File' && 'CSI driver supports volume ownership and permission changes'}
                                {fsGroupPolicy === 'None' && 'CSI driver does not support volume ownership/permissions'}
                            </p>
                        </div>
                    </DetailSection>
                )}

                {/* Token Requests */}
                {tokenRequests.length > 0 && (
                    <DetailSection title="Token Requests">
                        <div className="space-y-2">
                            {tokenRequests.map((request: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3">
                                    <div className="text-sm text-gray-300">{request.audience}</div>
                                    {request.expirationSeconds && (
                                        <div className="text-xs text-gray-500 mt-1">
                                            Expires in {request.expirationSeconds}s
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Created">
                        <span title={metadata.creationTimestamp}>
                            {formatAge(metadata.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={metadata.uid?.substring(0, 8) + '...'} copyValue={metadata.uid} />
                    </DetailRow>
                </DetailSection>

                {/* Labels */}
                <DetailSection title="Labels">
                    <LabelsDisplay labels={metadata.labels} />
                </DetailSection>

                {/* Annotations */}
                <DetailSection title="Annotations">
                    <AnnotationsDisplay annotations={metadata.annotations} />
                </DetailSection>
            </div>
        </div>
    );
}
