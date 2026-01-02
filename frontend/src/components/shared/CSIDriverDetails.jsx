import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';

const BooleanBadge = ({ value, label }) => {
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

export default function CSIDriverDetails({ csiDriver, tabContext }) {
    const metadata = csiDriver?.metadata || {};
    const spec = csiDriver?.spec || {};

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
    ];

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
        <DetailsPanel
            title={metadata.name}
            subtitle="CSI Driver"
        >
            <div className="space-y-6 p-4">
                {/* Basic Info */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Basic Information</h3>
                    <div className="grid grid-cols-2 gap-4">
                        {basicInfo.map(({ label, value }) => (
                            <div key={label}>
                                <dt className="text-xs text-gray-500">{label}</dt>
                                <dd className="text-sm text-gray-200 mt-0.5">{value ?? '-'}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Capabilities */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Capabilities</h3>
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
                </div>

                {/* Volume Lifecycle Modes */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Volume Lifecycle Modes</h3>
                    {volumeLifecycleModes.length === 0 ? (
                        <p className="text-sm text-gray-500">No modes specified (defaults to Persistent)</p>
                    ) : (
                        <div className="flex flex-wrap gap-2">
                            {volumeLifecycleModes.map((mode) => (
                                <span
                                    key={mode}
                                    className="inline-flex items-center px-2.5 py-1 rounded text-xs bg-blue-500/20 text-blue-400"
                                >
                                    {mode}
                                </span>
                            ))}
                        </div>
                    )}
                </div>

                {/* FS Group Policy */}
                {fsGroupPolicy && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">FS Group Policy</h3>
                        <div className="bg-gray-800/50 rounded-lg p-3">
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
                    </div>
                )}

                {/* Token Requests */}
                {tokenRequests.length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Token Requests</h3>
                        <div className="space-y-2">
                            {tokenRequests.map((request, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="text-sm text-gray-300">{request.audience}</div>
                                    {request.expirationSeconds && (
                                        <div className="text-xs text-gray-500 mt-1">
                                            Expires in {request.expirationSeconds}s
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    </div>
                )}

                {/* Labels */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Labels</h3>
                    <LabelsDisplay labels={metadata.labels} />
                </div>

                {/* Annotations */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Annotations</h3>
                    <AnnotationsDisplay annotations={metadata.annotations} />
                </div>
            </div>
        </DetailsPanel>
    );
}
