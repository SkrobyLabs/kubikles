import React from 'react';
import { PencilSquareIcon, ShareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailSection, DetailRow, LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

const WRITE_VERBS = new Set(['create', 'update', 'patch', 'delete', 'deletecollection']);

function VerbBadge({ verb }: { verb: string }) {
    if (verb === '*') {
        return (
            <span className="px-1.5 py-0.5 text-xs rounded bg-red-500/15 text-red-400 border border-red-500/30">
                {verb}
            </span>
        );
    }
    if (WRITE_VERBS.has(verb)) {
        return (
            <span className="px-1.5 py-0.5 text-xs rounded bg-orange-500/15 text-orange-400 border border-orange-500/30">
                {verb}
            </span>
        );
    }
    return (
        <span className="px-1.5 py-0.5 text-xs rounded bg-gray-500/10 text-gray-300 border border-gray-500/30">
            {verb}
        </span>
    );
}

export default function ClusterRoleDetails({ clusterRole, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = clusterRole?.metadata || {};
    const rules = clusterRole?.rules || [];
    const aggregationRule = clusterRole?.aggregationRule;
    const selectors = aggregationRule?.clusterRoleSelectors || [];

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-clusterrole-${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="clusterrole"
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-clusterrole-${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="clusterrole"
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
                        {name}
                    </div>
                    <span className="px-2 py-0.5 text-xs rounded bg-blue-500/10 text-blue-400 border border-blue-500/30">
                        {rules.length} rule{rules.length !== 1 ? 's' : ''}
                    </span>
                    {selectors.length > 0 && (
                        <span className="px-2 py-0.5 text-xs rounded bg-purple-500/10 text-purple-400 border border-purple-500/30">
                            Aggregated
                        </span>
                    )}
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
                    <DetailRow label="Scope" value="Cluster-wide" />
                    <DetailRow label="Created">
                        <span title={metadata.creationTimestamp}>
                            {formatAge(metadata.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                </DetailSection>

                {/* Aggregation Rule */}
                {selectors.length > 0 && (
                    <DetailSection title="Aggregation Rule">
                        <p className="text-xs text-gray-400 mb-2">
                            This ClusterRole aggregates rules from other ClusterRoles matching these selectors:
                        </p>
                        <div className="space-y-2">
                            {selectors.map((sel: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3">
                                    <div className="flex flex-wrap gap-1">
                                        {Object.entries(sel.matchLabels || {}).map(([key, value]) => (
                                            <span key={key} className="px-1.5 py-0.5 text-xs rounded bg-purple-500/10 text-purple-400 border border-purple-500/30">
                                                {key}={String(value)}
                                            </span>
                                        ))}
                                    </div>
                                </div>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Rules */}
                <DetailSection title={`Rules (${rules.length})`}>
                    {rules.length === 0 ? (
                        <span className="text-gray-500">No rules defined</span>
                    ) : (
                        <div className="space-y-3">
                            {rules.map((rule: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3">
                                    <div className="grid grid-cols-[100px_1fr] gap-y-2 text-sm">
                                        <span className="text-xs font-medium text-gray-500 uppercase">API Groups</span>
                                        <div className="flex flex-wrap gap-1">
                                            {(rule.apiGroups || []).map((g: string, i: number) => (
                                                <span key={i} className="px-1.5 py-0.5 text-xs rounded bg-gray-500/10 text-gray-300 border border-gray-500/30">
                                                    {g === '' ? 'core' : g}
                                                </span>
                                            ))}
                                        </div>

                                        <span className="text-xs font-medium text-gray-500 uppercase">Resources</span>
                                        <div className="flex flex-wrap gap-1">
                                            {(rule.resources || []).map((r: string, i: number) => (
                                                <span key={i} className="px-1.5 py-0.5 text-xs rounded bg-blue-500/10 text-blue-400 border border-blue-500/30">
                                                    {r}
                                                </span>
                                            ))}
                                        </div>

                                        <span className="text-xs font-medium text-gray-500 uppercase">Verbs</span>
                                        <div className="flex flex-wrap gap-1">
                                            {(rule.verbs || []).map((v: string, i: number) => (
                                                <VerbBadge key={i} verb={v} />
                                            ))}
                                        </div>

                                        {rule.resourceNames && rule.resourceNames.length > 0 && (
                                            <>
                                                <span className="text-xs font-medium text-gray-500 uppercase">Names</span>
                                                <div className="flex flex-wrap gap-1">
                                                    {rule.resourceNames.map((n: string, i: number) => (
                                                        <span key={i} className="px-1.5 py-0.5 text-xs rounded bg-purple-500/10 text-purple-400 border border-purple-500/30">
                                                            {n}
                                                        </span>
                                                    ))}
                                                </div>
                                            </>
                                        )}
                                    </div>
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
