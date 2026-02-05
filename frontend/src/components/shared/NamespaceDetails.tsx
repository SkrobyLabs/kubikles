import React, { useState, useEffect, useMemo } from 'react';
import { PencilSquareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context';
import { useUI } from '../../context';
import { GetNamespaceResourceCounts } from '../../../wailsjs/go/main/App';
import { formatAge } from '../../utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, ResourceCountBadge } from './DetailComponents';
import { LazyYamlEditor as YamlEditor } from '../lazy';
import NamespaceMetricsTab from './NamespaceMetricsTab';

const TAB_BASIC = 'basic';
const TAB_METRICS = 'metrics';

export default function NamespaceDetails({ namespace, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch, setSelectedNamespaces, getDetailTab, setDetailTab } = useUI();
    const [resourceCounts, setResourceCounts] = useState(null);
    const [loadingCounts, setLoadingCounts] = useState(true);
    const activeTab = getDetailTab('namespace', TAB_BASIC);
    const setActiveTab = (tab) => setDetailTab('namespace', tab);

    // Check if this tab is stale
    const isStale = tabContext && tabContext !== currentContext;

    const name = namespace.metadata?.name;

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_METRICS, label: 'Metrics' },
    ], []);
    const labels = namespace.metadata?.labels || {};
    const annotations = namespace.metadata?.annotations || {};
    const status = namespace.status?.phase || 'Unknown';

    // Fetch resource counts
    useEffect(() => {
        if (isStale || !name) return;

        const fetchCounts = async () => {
            setLoadingCounts(true);
            try {
                const counts = await GetNamespaceResourceCounts(name);
                setResourceCounts(counts);
            } catch (err) {
                console.error('Failed to fetch resource counts:', err);
            } finally {
                setLoadingCounts(false);
            }
        };

        fetchCounts();
    }, [name, isStale]);

    const handleEditYaml = () => {
        const tabId = `yaml-${namespace.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="namespace"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleNavigateToResource = (viewName) => {
        setSelectedNamespaces([name]);
        navigateWithSearch(viewName, '');
    };

    const getStatusVariant = (status) => {
        switch (status) {
            case 'Active': return 'success';
            case 'Terminating': return 'warning';
            default: return 'default';
        }
    };

    const resourceTypes = [
        { key: 'pods', label: 'Pods', view: 'pods' },
        { key: 'deployments', label: 'Deployments', view: 'deployments' },
        { key: 'statefulsets', label: 'StatefulSets', view: 'statefulsets' },
        { key: 'daemonsets', label: 'DaemonSets', view: 'daemonsets' },
        { key: 'replicasets', label: 'ReplicaSets', view: 'replicasets' },
        { key: 'jobs', label: 'Jobs', view: 'jobs' },
        { key: 'cronjobs', label: 'CronJobs', view: 'cronjobs' },
        { key: 'services', label: 'Services', view: 'services' },
        { key: 'ingresses', label: 'Ingresses', view: 'ingresses' },
        { key: 'configmaps', label: 'ConfigMaps', view: 'configmaps' },
        { key: 'secrets', label: 'Secrets', view: 'secrets' },
        { key: 'pvcs', label: 'PVCs', view: 'pvcs' },
    ];

    const renderTabContent = () => {
        switch (activeTab) {
            case TAB_BASIC:
                return (
                    <div className="h-full overflow-auto p-4">
                        {/* Resource Counts */}
                        <DetailSection title="Resources">
                            {loadingCounts ? (
                                <div className="text-gray-500 text-sm">Loading resource counts...</div>
                            ) : resourceCounts ? (
                                <div className="grid grid-cols-4 gap-3">
                                    {resourceTypes.map(({ key, label, view }) => (
                                        <ResourceCountBadge
                                            key={key}
                                            count={resourceCounts[key] || 0}
                                            label={label}
                                            onClick={() => handleNavigateToResource(view)}
                                        />
                                    ))}
                                </div>
                            ) : (
                                <div className="text-gray-500 text-sm">Failed to load resource counts</div>
                            )}
                        </DetailSection>

                        {/* Basic Info */}
                        <DetailSection title="Details">
                            <DetailRow label="Name" value={name} />
                            <DetailRow label="Status">
                                <StatusBadge status={status} variant={getStatusVariant(status)} />
                            </DetailRow>
                            <DetailRow label="Created">
                                <span title={namespace.metadata?.creationTimestamp}>
                                    {formatAge(namespace.metadata?.creationTimestamp)} ago
                                </span>
                            </DetailRow>
                            <DetailRow label="UID" value={namespace.metadata?.uid} />
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
                );
            case TAB_METRICS:
                return (
                    <NamespaceMetricsTab
                        namespace={namespace}
                        isStale={isStale}
                    />
                );
            default:
                return null;
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
                    <StatusBadge status={status} variant={getStatusVariant(status)} />
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
            <div className="flex-1 overflow-hidden">
                {renderTabContent()}
            </div>
        </div>
    );
}
