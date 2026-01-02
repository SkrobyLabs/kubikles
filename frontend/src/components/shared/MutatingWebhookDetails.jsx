import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';

export default function MutatingWebhookDetails({ webhook, tabContext }) {
    const metadata = webhook?.metadata || {};
    const webhooks = webhook?.webhooks || [];

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Webhooks', value: webhooks.length.toString() },
    ];

    const formatRules = (rules) => {
        if (!rules || rules.length === 0) return 'None';
        return rules.map(rule => {
            const ops = (rule.operations || []).join(', ');
            const resources = (rule.resources || []).join(', ');
            const apiGroups = (rule.apiGroups || ['']).map(g => g || 'core').join(', ');
            return `${ops} on ${apiGroups}/${resources}`;
        }).join('; ');
    };

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="Mutating Webhook Configuration"
        >
            <div className="space-y-6 p-4">
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
                            {webhooks.map((wh, idx) => (
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
                </div>

                {/* Labels */}
                {metadata.labels && Object.keys(metadata.labels).length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Labels</h3>
                        <div className="flex flex-wrap gap-2">
                            {Object.entries(metadata.labels).map(([key, value]) => (
                                <span
                                    key={key}
                                    className="inline-flex items-center px-2 py-1 rounded text-xs bg-gray-700 text-gray-300"
                                >
                                    {key}: {value}
                                </span>
                            ))}
                        </div>
                    </div>
                )}
            </div>
        </DetailsPanel>
    );
}
