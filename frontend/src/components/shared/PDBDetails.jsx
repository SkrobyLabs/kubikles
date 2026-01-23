import React from 'react';
import { PencilSquareIcon, ShareIcon, ShieldExclamationIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import { formatAge } from '../../utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function PDBDetails({ pdb, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = pdb?.metadata || {};
    const spec = pdb?.spec || {};
    const status = pdb?.status || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-pdb-${pdb.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldExclamationIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="pdb"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-pdb-${pdb.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldExclamationIcon,
            content: (
                <DependencyGraph
                    resourceType="pdb"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

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
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {namespace}/{name}
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
            <div className="space-y-6">
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
            </div>
        </div>
    );
}
