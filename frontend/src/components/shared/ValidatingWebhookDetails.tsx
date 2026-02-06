import React from 'react';
import { PencilSquareIcon, ShareIcon, ShieldCheckIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function ValidatingWebhookDetails({ webhook, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = webhook?.metadata || {};
    const webhooks = webhook?.webhooks || [];

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-validatingwebhook-${webhook.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldCheckIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="validatingwebhookconfiguration"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-validatingwebhook-${webhook.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldCheckIcon,
            content: (
                <DependencyGraph
                    resourceType="validatingwebhookconfiguration"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Webhooks', value: webhooks.length.toString() },
    ];

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
            <div className="space-y-6">
                {/* Basic Info */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Basic Information</h3>
                    <div className="grid grid-cols-2 gap-4">
                        {basicInfo.map(({ label, value }) => (
                            <div key={label}>
                                <dt className="text-xs text-gray-500">{label}</dt>
                                <dd className="text-sm text-gray-200 mt-0.5">{value || '-'}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Webhooks */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Webhooks ({webhooks.length})</h3>
                    {webhooks.length === 0 ? (
                        <p className="text-sm text-gray-500">No webhooks configured</p>
                    ) : (
                        <div className="space-y-3">
                            {webhooks.map((wh: any, idx: number) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="text-sm font-medium text-gray-200 mb-2">{wh.name}</div>
                                    <div className="grid grid-cols-2 gap-2 text-xs">
                                        <div>
                                            <span className="text-gray-500">Failure Policy:</span>
                                            <span className={`ml-1 ${wh.failurePolicy === 'Fail' ? 'text-red-400' : 'text-yellow-400'}`}>
                                                {wh.failurePolicy || 'Fail'}
                                            </span>
                                        </div>
                                        <div>
                                            <span className="text-gray-500">Side Effects:</span>
                                            <span className="ml-1 text-gray-300">{wh.sideEffects || 'Unknown'}</span>
                                        </div>
                                        <div>
                                            <span className="text-gray-500">Timeout:</span>
                                            <span className="ml-1 text-gray-300">{wh.timeoutSeconds || 10}s</span>
                                        </div>
                                        <div>
                                            <span className="text-gray-500">Match Policy:</span>
                                            <span className="ml-1 text-gray-300">{wh.matchPolicy || 'Equivalent'}</span>
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
                </div>

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
