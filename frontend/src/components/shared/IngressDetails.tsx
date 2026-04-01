import React from 'react';
import { PencilSquareIcon, ShareIcon, GlobeAltIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function IngressDetails({ ingress, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch } = useUI();

    const isStale = tabContext && tabContext !== currentContext;

    const name = ingress.metadata?.name;
    const namespace = ingress.metadata?.namespace;
    const labels = ingress.metadata?.labels || {};
    const annotations = ingress.metadata?.annotations || {};
    const spec = ingress.spec || {};
    const status = ingress.status || {};

    const ingressClassName = spec.ingressClassName || annotations['kubernetes.io/ingress.class'] || '-';
    const rules = spec.rules || [];
    const tls = spec.tls || [];
    const defaultBackend = spec.defaultBackend;
    const loadBalancer = status.loadBalancer?.ingress || [];

    // Determine ingress status - "Active" only if we have addresses assigned
    const getIngressStatus = () => {
        if (loadBalancer.length > 0) {
            const hasAddress = loadBalancer.some((lb: any) => lb.ip || lb.hostname);
            if (hasAddress) return { status: 'Active', variant: 'success' };
        }
        // Check if there are any rules defined
        if (rules.length === 0 && !defaultBackend) {
            return { status: 'No Rules', variant: 'error' };
        }
        return { status: 'Pending', variant: 'warning' };
    };

    const ingressStatus = getIngressStatus();

    const handleEditYaml = () => {
        const tabId = `yaml-ingress-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="ingress"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-ingress-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="ingress"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleViewService = (serviceName: any) => {
        navigateWithSearch('services', `name:"${serviceName}" namespace:"${namespace}"`);
    };

    const renderBackend = (backend: any) => {
        if (!backend) return <span className="text-gray-500">-</span>;

        if (backend.service) {
            const serviceName = backend.service.name;
            const port = backend.service.port?.number || backend.service.port?.name || '-';
            return (
                <button
                    onClick={() => handleViewService(serviceName)}
                    className="text-primary hover:text-primary/80 hover:underline"
                >
                    {serviceName}:{port}
                </button>
            );
        }

        if (backend.resource) {
            return (
                <span className="text-gray-300">
                    {backend.resource.kind}/{backend.resource.name}
                </span>
            );
        }

        return <span className="text-gray-500">-</span>;
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {namespace}/{name}
                    </div>
                    <StatusBadge
                        status={ingressStatus.status}
                        variant={ingressStatus.variant}
                    />
                    <StatusBadge status={ingressClassName} variant="default" />
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
                {/* Load Balancer Status */}
                {loadBalancer.length > 0 && (
                    <DetailSection title="Load Balancer">
                        <div className="space-y-1.5">
                            {loadBalancer.map((lb: any, idx: number) => (
                                <div key={idx} className="flex items-center gap-2">
                                    <GlobeAltIcon className="h-4 w-4 text-gray-400" />
                                    <CopyableLabel value={lb.ip || lb.hostname} />
                                </div>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Rules */}
                <DetailSection title="Rules">
                    {rules.length > 0 ? (
                        <div className="space-y-4">
                            {rules.map((rule: any, ruleIdx: number) => (
                                <div key={ruleIdx} className="p-3 bg-background-dark rounded border border-border">
                                    <div className="flex items-center gap-2 mb-2">
                                        <span className="text-sm font-medium text-gray-300">
                                            {rule.host || '*'}
                                        </span>
                                        {tls.some((t: any) => t.hosts?.includes(rule.host)) && (
                                            <StatusBadge status="TLS" variant="success" />
                                        )}
                                    </div>
                                    {rule.http?.paths && rule.http.paths.length > 0 && (
                                        <div className="space-y-1.5 ml-4">
                                            {rule.http.paths.map((path: any, pathIdx: number) => (
                                                <div key={pathIdx} className="flex items-center gap-4 text-sm">
                                                    <span className="font-mono text-gray-400 min-w-[100px]">
                                                        {path.path || '/'}
                                                    </span>
                                                    <span className="text-xs text-gray-500">
                                                        ({path.pathType || 'Prefix'})
                                                    </span>
                                                    <span className="text-gray-500">→</span>
                                                    {renderBackend(path.backend)}
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    ) : (
                        <span className="text-gray-500">No rules defined</span>
                    )}
                </DetailSection>

                {/* Default Backend */}
                {defaultBackend && (
                    <DetailSection title="Default Backend">
                        {renderBackend(defaultBackend)}
                    </DetailSection>
                )}

                {/* TLS Configuration */}
                {tls.length > 0 && (
                    <DetailSection title="TLS">
                        <div className="space-y-2">
                            {tls.map((tlsEntry: any, idx: number) => (
                                <div key={idx} className="p-2 bg-background-dark rounded border border-border">
                                    <div className="flex items-center gap-2 mb-1">
                                        <span className="text-sm text-gray-300">Secret:</span>
                                        {tlsEntry.secretName ? (
                                            <button
                                                onClick={() => navigateWithSearch('secrets', `name:"${tlsEntry.secretName}" namespace:"${namespace}"`)}
                                                className="text-primary hover:text-primary/80 hover:underline text-sm"
                                            >
                                                {tlsEntry.secretName}
                                            </button>
                                        ) : (
                                            <span className="text-gray-500 text-sm">-</span>
                                        )}
                                    </div>
                                    {tlsEntry.hosts && tlsEntry.hosts.length > 0 && (
                                        <div className="text-xs text-gray-400">
                                            Hosts: {tlsEntry.hosts.join(', ')}
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Ingress Class" value={ingressClassName} />
                    <DetailRow label="Created">
                        <span title={ingress.metadata?.creationTimestamp}>
                            {formatAge(ingress.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={ingress.metadata?.uid?.substring(0, 8) + '...'} copyValue={ingress.metadata?.uid} />
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
        </div>
    );
}
