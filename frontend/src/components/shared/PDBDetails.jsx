import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';

export default function PDBDetails({ pdb, tabContext }) {
    const metadata = pdb?.metadata || {};
    const spec = pdb?.spec || {};
    const status = pdb?.status || {};

    const getSelector = () => {
        const selector = spec.selector;
        if (!selector) return '-';
        const labels = selector.matchLabels || {};
        if (Object.keys(labels).length === 0) return 'All Pods';
        return Object.entries(labels)
            .map(([k, v]) => `${k}=${v}`)
            .join(', ');
    };

    const getBudgetValue = () => {
        if (spec.minAvailable !== undefined) {
            return { type: 'minAvailable', value: spec.minAvailable };
        }
        if (spec.maxUnavailable !== undefined) {
            return { type: 'maxUnavailable', value: spec.maxUnavailable };
        }
        return { type: 'none', value: '-' };
    };

    const budget = getBudgetValue();

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Selector', value: getSelector() },
        { label: 'Budget Type', value: budget.type === 'minAvailable' ? 'Min Available' : 'Max Unavailable' },
        { label: 'Budget Value', value: budget.value },
    ];

    const statusInfo = [
        { label: 'Current Healthy', value: status.currentHealthy ?? '-' },
        { label: 'Desired Healthy', value: status.desiredHealthy ?? '-' },
        { label: 'Disruptions Allowed', value: status.disruptionsAllowed ?? '-' },
        { label: 'Expected Pods', value: status.expectedPods ?? '-' },
        { label: 'Observed Generation', value: status.observedGeneration ?? '-' },
    ];

    const conditions = status.conditions || [];

    const getHealthStatus = () => {
        const current = status.currentHealthy ?? 0;
        const desired = status.desiredHealthy ?? 0;
        if (current >= desired) {
            return { text: 'Healthy', color: 'text-green-400' };
        }
        return { text: 'Unhealthy', color: 'text-red-400' };
    };

    const healthStatus = getHealthStatus();

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="Pod Disruption Budget"
        >
            <div className="space-y-6 p-4">
                {/* Status Summary */}
                <div className="bg-gray-800/50 rounded-lg p-4 flex items-center justify-between">
                    <div>
                        <span className="text-sm text-gray-400">Status:</span>
                        <span className={`ml-2 text-lg font-medium ${healthStatus.color}`}>{healthStatus.text}</span>
                    </div>
                    <div className="text-right">
                        <div className="text-2xl font-bold text-gray-200">{status.disruptionsAllowed ?? 0}</div>
                        <div className="text-xs text-gray-500">Disruptions Allowed</div>
                    </div>
                </div>

                {/* Basic Info */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Configuration</h3>
                    <div className="grid grid-cols-2 gap-4">
                        {basicInfo.map(({ label, value }) => (
                            <div key={label}>
                                <dt className="text-xs text-gray-500">{label}</dt>
                                <dd className="text-sm text-gray-200 mt-0.5">{value ?? '-'}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Status Info */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Current Status</h3>
                    <div className="grid grid-cols-2 gap-4">
                        {statusInfo.map(({ label, value }) => (
                            <div key={label}>
                                <dt className="text-xs text-gray-500">{label}</dt>
                                <dd className="text-sm text-gray-200 mt-0.5">{value}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Conditions */}
                {conditions.length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Conditions</h3>
                        <div className="space-y-2">
                            {conditions.map((condition, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="flex justify-between items-start">
                                        <span className={`text-sm ${condition.status === 'True' ? 'text-green-400' : 'text-gray-300'}`}>
                                            {condition.type}
                                        </span>
                                        <span className={`text-xs ${condition.status === 'True' ? 'text-green-500' : 'text-gray-500'}`}>
                                            {condition.status}
                                        </span>
                                    </div>
                                    {condition.message && (
                                        <p className="text-xs text-gray-500 mt-1">{condition.message}</p>
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
