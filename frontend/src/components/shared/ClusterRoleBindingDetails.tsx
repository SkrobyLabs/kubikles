import React from 'react';
import { PencilSquareIcon, ShareIcon, UserIcon, UserGroupIcon, CubeIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailSection, DetailRow, LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

function SubjectIcon({ kind }: { kind: string }) {
    switch (kind) {
        case 'User':
            return <UserIcon className="w-4 h-4 text-blue-400" />;
        case 'Group':
            return <UserGroupIcon className="w-4 h-4 text-purple-400" />;
        case 'ServiceAccount':
            return <CubeIcon className="w-4 h-4 text-green-400" />;
        default:
            return <UserIcon className="w-4 h-4 text-gray-400" />;
    }
}

export default function ClusterRoleBindingDetails({ clusterRoleBinding, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = clusterRoleBinding?.metadata || {};
    const subjects = clusterRoleBinding?.subjects || [];
    const roleRef = clusterRoleBinding?.roleRef || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-clusterrolebinding-${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="clusterrolebinding"
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-clusterrolebinding-${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="clusterrolebinding"
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
                        {subjects.length} subject{subjects.length !== 1 ? 's' : ''}
                    </span>
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

                {/* Role Reference */}
                <DetailSection title="Role Reference">
                    <DetailRow label="Kind" value={roleRef.kind} />
                    <DetailRow label="Name" value={roleRef.name} />
                    <DetailRow label="API Group" value={roleRef.apiGroup || 'rbac.authorization.k8s.io'} />
                </DetailSection>

                {/* Subjects */}
                <DetailSection title={`Subjects (${subjects.length})`}>
                    {subjects.length === 0 ? (
                        <span className="text-gray-500">No subjects defined</span>
                    ) : (
                        <div className="space-y-2">
                            {subjects.map((subject: any, idx: number) => (
                                <div key={idx} className="flex items-center gap-3 bg-background-dark rounded border border-border p-3">
                                    <SubjectIcon kind={subject.kind} />
                                    <div className="flex-1 min-w-0">
                                        <div className="flex items-center gap-2">
                                            <span className="text-sm text-gray-200 font-medium">{subject.name}</span>
                                            <span className="px-1.5 py-0.5 text-xs rounded bg-gray-500/10 text-gray-400 border border-gray-500/30">
                                                {subject.kind}
                                            </span>
                                        </div>
                                        {subject.namespace && (
                                            <div className="text-xs text-gray-500 mt-0.5">
                                                Namespace: {subject.namespace}
                                            </div>
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
