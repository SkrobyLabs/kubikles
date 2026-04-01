import React from 'react';
import { PencilSquareIcon, ShareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailSection, DetailRow, LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function ServiceAccountDetails({ serviceAccount, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = serviceAccount?.metadata || {};
    const secrets = serviceAccount?.secrets || [];
    const imagePullSecrets = serviceAccount?.imagePullSecrets || [];
    const automountToken = serviceAccount?.automountServiceAccountToken;

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-serviceaccount-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="serviceaccount"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-serviceaccount-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="serviceaccount"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {namespace}/{name}
                    </div>
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        <button
                            onClick={handleEditYaml}
                            className={`p-1.5 rounded transition-colors ${isStale ? 'text-gray-600 cursor-not-allowed' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
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
            <div className="h-full overflow-auto p-4">
                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Created">
                        <span title={metadata.creationTimestamp}>
                            {formatAge(metadata.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="Automount Token">
                        {automountToken === false ? (
                            <span className="text-yellow-400">Disabled</span>
                        ) : (
                            <span className="text-green-400">Enabled (default)</span>
                        )}
                    </DetailRow>
                </DetailSection>

                {/* Secrets */}
                <DetailSection title={`Secrets (${secrets.length})`}>
                    {secrets.length === 0 ? (
                        <span className="text-gray-500">No secrets associated</span>
                    ) : (
                        <div className="space-y-2">
                            {secrets.map((secret: any, idx: number) => (
                                <div key={idx} className="flex items-center gap-2 bg-background-dark rounded border border-border p-3">
                                    <span className="text-sm text-gray-200">{secret.name}</span>
                                </div>
                            ))}
                        </div>
                    )}
                </DetailSection>

                {/* Image Pull Secrets */}
                <DetailSection title={`Image Pull Secrets (${imagePullSecrets.length})`}>
                    {imagePullSecrets.length === 0 ? (
                        <span className="text-gray-500">No image pull secrets</span>
                    ) : (
                        <div className="space-y-2">
                            {imagePullSecrets.map((secret: any, idx: number) => (
                                <div key={idx} className="flex items-center gap-2 bg-background-dark rounded border border-border p-3">
                                    <span className="text-sm text-gray-200">{secret.name}</span>
                                </div>
                            ))}
                        </div>
                    )}
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
