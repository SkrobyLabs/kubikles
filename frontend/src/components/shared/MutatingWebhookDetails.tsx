import React from 'react';
import { PencilSquareIcon, ShareIcon, FingerPrintIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function MutatingWebhookDetails({ webhook, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = webhook?.metadata || {};
    const webhooks = webhook?.webhooks || [];

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-mutatingwebhook-${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: FingerPrintIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="mutatingwebhookconfiguration"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-mutatingwebhook-${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: FingerPrintIcon,
            content: (
                <DependencyGraph
                    resourceType="mutatingwebhookconfiguration"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const formatRules = (rules: any) => {
        if (!rules || rules.length === 0) return 'None';
        return rules.map((rule: any) => {
            const ops = (rule.operations || []).join(', ');
            const resources = (rule.resources || []).join(', ');
            const apiGroups = (rule.apiGroups || ['']).map((g: any) => g || 'core').join(', ');
            return `${ops} on ${apiGroups}/${resources}`;
        }).join('; ');
    };

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
                {/* Webhooks */}
                <DetailSection title={`Webhooks (${webhooks.length})`}>
                    {webhooks.length === 0 ? (
                        <span className="text-gray-500">No webhooks configured</span>
                    ) : (
                        <div className="space-y-3">
                            {webhooks.map((wh: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3">
                                    <div className="text-sm font-medium text-gray-200 mb-2">{wh.name}</div>
                                    <div className="grid grid-cols-2 gap-2 text-xs">
                                        <div>
                                            <span className="text-gray-500">Failure Policy:</span>
                                            <span className={`ml-1 ${wh.failurePolicy === 'Fail' ? 'text-red-400' : 'text-yellow-400'}`}>
                                                {wh.failurePolicy || 'Fail'}
                                            </span>
                                        </div>
                                        <div>
                                            <span className="text-gray-500">Reinvocation:</span>
                                            <span className="ml-1 text-gray-300">{wh.reinvocationPolicy || 'Never'}</span>
                                        </div>
                                        <div>
                                            <span className="text-gray-500">Side Effects:</span>
                                            <span className="ml-1 text-gray-300">{wh.sideEffects || 'Unknown'}</span>
                                        </div>
                                        <div>
                                            <span className="text-gray-500">Timeout:</span>
                                            <span className="ml-1 text-gray-300">{wh.timeoutSeconds || 10}s</span>
                                        </div>
                                    </div>
                                    {wh.clientConfig && (
                                        <div className="mt-2 text-xs">
                                            <span className="text-gray-500">Target:</span>
                                            <span className="ml-1 text-gray-300 font-mono">
                                                {wh.clientConfig.service
                                                    ? `${wh.clientConfig.service.namespace}/${wh.clientConfig.service.name}:${wh.clientConfig.service.port || 443}`
                                                    : wh.clientConfig.url || 'N/A'}
                                            </span>
                                        </div>
                                    )}
                                    <div className="mt-2 text-xs">
                                        <span className="text-gray-500">Rules:</span>
                                        <span className="ml-1 text-gray-300">{formatRules(wh.rules)}</span>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </DetailSection>

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Webhooks" value={webhooks.length} />
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
