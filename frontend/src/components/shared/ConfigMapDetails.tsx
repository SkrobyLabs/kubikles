import React, { useState } from 'react';
import { PencilSquareIcon, ShareIcon, DocumentDuplicateIcon, ChevronDownIcon, ChevronRightIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

const TAB_INFO = 'info';
const TAB_DATA = 'data';

function ConfigMapKeyValue({ keyName, value, isExpanded, onToggle }: { keyName: any; value: any; isExpanded: any; onToggle: any }) {
    const [copied, setCopied] = useState(false);
    const isMultiline = value && value.includes('\n');
    const displayValue = isMultiline ? value : (value || '');

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(value || '');
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        } catch (err: any) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <div className="border border-border rounded bg-background-dark">
            <div
                className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-white/5"
                onClick={onToggle}
            >
                {isMultiline ? (
                    isExpanded ? (
                        <ChevronDownIcon className="h-4 w-4 text-gray-400" />
                    ) : (
                        <ChevronRightIcon className="h-4 w-4 text-gray-400" />
                    )
                ) : (
                    <div className="w-4" />
                )}
                <span className="font-mono text-sm text-gray-300">{keyName}</span>
                <button
                    onClick={(e) => {
                        e.stopPropagation();
                        handleCopy();
                    }}
                    className="ml-auto p-1 text-gray-500 hover:text-gray-300 transition-colors"
                    title="Copy value"
                >
                    <DocumentDuplicateIcon className={`h-4 w-4 ${copied ? 'text-green-400' : ''}`} />
                </button>
            </div>
            {(isExpanded || !isMultiline) && (
                <div className="px-3 py-2 border-t border-border/50">
                    <pre className="text-xs text-gray-400 whitespace-pre-wrap font-mono overflow-x-auto">
                        {displayValue || <span className="text-gray-600 italic">(empty)</span>}
                    </pre>
                </div>
            )}
        </div>
    );
}

function ConfigMapInfoTab({ configMap }: { configMap: any }) {
    const name = configMap.metadata?.name;
    const namespace = configMap.metadata?.namespace;
    const labels = configMap.metadata?.labels || {};
    const annotations = configMap.metadata?.annotations || {};
    const data = configMap.data || {};
    const binaryData = configMap.binaryData || {};

    const dataKeys = Object.keys(data);
    const binaryKeys = Object.keys(binaryData);

    return (
        <div className="h-full overflow-auto p-4">
            {/* Summary */}
            <DetailSection title="Summary">
                <div className="grid grid-cols-2 gap-4 mb-2">
                    <div className="text-center p-3 bg-background-dark rounded border border-border">
                        <div className="text-2xl font-bold text-gray-200">{dataKeys.length}</div>
                        <div className="text-xs text-gray-500">Data Keys</div>
                    </div>
                    <div className="text-center p-3 bg-background-dark rounded border border-border">
                        <div className="text-2xl font-bold text-gray-200">{binaryKeys.length}</div>
                        <div className="text-xs text-gray-500">Binary Keys</div>
                    </div>
                </div>
            </DetailSection>

            {/* Details */}
            <DetailSection title="Details">
                <DetailRow label="Name" value={name} />
                <DetailRow label="Namespace" value={namespace} />
                <DetailRow label="Created">
                    <span title={configMap.metadata?.creationTimestamp}>
                        {formatAge(configMap.metadata?.creationTimestamp)} ago
                    </span>
                </DetailRow>
                <DetailRow label="UID">
                    <CopyableLabel value={configMap.metadata?.uid?.substring(0, 8) + '...'} copyValue={configMap.metadata?.uid} />
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
    );
}

function ConfigMapDataTab({ configMap }: { configMap: any }) {
    const [expandedKeys, setExpandedKeys] = useState<Record<string, boolean>>({});
    const data = configMap.data || {};
    const binaryData = configMap.binaryData || {};

    const dataKeys = Object.keys(data);
    const binaryKeys = Object.keys(binaryData);

    const toggleKey = (key: string) => {
        setExpandedKeys(prev => ({
            ...prev,
            [key]: !prev[key]
        }));
    };

    return (
        <div className="h-full overflow-auto p-4">
            {/* Data */}
            <DetailSection title={`Data (${dataKeys.length})`}>
                {dataKeys.length > 0 ? (
                    <div className="space-y-2">
                        {dataKeys.map((key: any) => (
                            <ConfigMapKeyValue
                                key={key}
                                keyName={key}
                                value={data[key]}
                                isExpanded={expandedKeys[key] || false}
                                onToggle={() => toggleKey(key)}
                            />
                        ))}
                    </div>
                ) : (
                    <span className="text-gray-500">No data</span>
                )}
            </DetailSection>

            {/* Binary Data */}
            {binaryKeys.length > 0 && (
                <DetailSection title={`Binary Data (${binaryKeys.length})`}>
                    <div className="space-y-1.5">
                        {binaryKeys.map((key: any) => (
                            <div key={key} className="flex items-center gap-2 px-3 py-2 bg-background-dark rounded border border-border">
                                <span className="font-mono text-sm text-gray-300">{key}</span>
                                <span className="text-xs text-gray-500">
                                    ({binaryData[key] ? Math.ceil(binaryData[key].length * 0.75) : 0} bytes)
                                </span>
                            </div>
                        ))}
                    </div>
                </DetailSection>
            )}
        </div>
    );
}

export default function ConfigMapDetails({ configMap, tabContext = '' }: { configMap: any; tabContext?: string }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, getDetailTab, setDetailTab } = useUI();
    const activeTab = getDetailTab('configmap', TAB_INFO);
    const setActiveTab = (tab: string) => setDetailTab('configmap', tab);

    const isStale = tabContext && tabContext !== currentContext;

    const name = configMap.metadata?.name;
    const namespace = configMap.metadata?.namespace;
    const data = configMap.data || {};
    const binaryData = configMap.binaryData || {};

    const totalKeys = Object.keys(data).length + Object.keys(binaryData).length;

    const handleEditYaml = () => {
        const tabId = `yaml-configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="configmap"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="configmap"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const tabs = [
        { id: TAB_INFO, label: 'Info' },
        { id: TAB_DATA, label: `Data (${totalKeys})` },
    ];

    const renderTabContent = () => {
        switch (activeTab) {
            case TAB_INFO:
                return <ConfigMapInfoTab configMap={configMap} />;
            case TAB_DATA:
                return <ConfigMapDataTab configMap={configMap} />;
            default:
                return null;
        }
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {namespace}/{name}
                    </div>
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
                            disabled={!!isStale}
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
            <div className="flex-1 overflow-hidden">
                {renderTabContent()}
            </div>
        </div>
    );
}
