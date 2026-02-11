import React, { useCallback } from 'react';
import { ShieldCheckIcon } from '@heroicons/react/24/outline';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { getPodController } from '~/utils/k8s-helpers';
import { formatAge } from '~/utils/formatting';
import { getOwnerViewId } from '~/utils/owner-navigation';
import { CopyableLabel, DetailRow } from './DetailComponents';
import Tooltip from './Tooltip';

function formatMatchExpr(expr: any): string {
    if (!expr?.key) return String(expr);
    const { key, operator, values } = expr;
    if (operator === 'Exists') return `${key} Exists`;
    if (operator === 'DoesNotExist') return `${key} DoesNotExist`;
    if (operator === 'Gt') return `${key} > ${values?.[0]}`;
    if (operator === 'Lt') return `${key} < ${values?.[0]}`;
    return `${key} ${operator} [${(values || []).join(', ')}]`;
}

function renderPodAffinityRules(pa: any): React.ReactNode[] {
    const rules: React.ReactNode[] = [];

    pa.requiredDuringSchedulingIgnoredDuringExecution?.forEach((term: any, ti: number) => {
        const labels: React.ReactNode[] = [];
        if (term.topologyKey)
            labels.push(<CopyableLabel key="tk" value={`topology: ${term.topologyKey}`} copyValue={term.topologyKey} />);
        if (term.labelSelector?.matchLabels)
            Object.entries(term.labelSelector.matchLabels).sort(([a], [b]) => a.localeCompare(b))
                .forEach(([k, v]) => labels.push(<CopyableLabel key={`ml-${k}`} value={`${k}=${v}`} />));
        (term.labelSelector?.matchExpressions || []).forEach((expr: any, i: number) =>
            labels.push(<CopyableLabel key={`me-${i}`} value={formatMatchExpr(expr)} copyValue={expr.key} />));
        if (labels.length > 0)
            rules.push(
                <div key={`req-${ti}`} className="flex flex-wrap items-center gap-1.5">
                    <span className="text-xs text-gray-500">required:</span>
                    {labels}
                </div>
            );
    });

    pa.preferredDuringSchedulingIgnoredDuringExecution?.forEach((pref: any, pi: number) => {
        const term = pref.podAffinityTerm;
        if (!term) return;
        const labels: React.ReactNode[] = [];
        if (term.topologyKey)
            labels.push(<CopyableLabel key="tk" value={`topology: ${term.topologyKey}`} copyValue={term.topologyKey} />);
        if (term.labelSelector?.matchLabels)
            Object.entries(term.labelSelector.matchLabels).sort(([a], [b]) => a.localeCompare(b))
                .forEach(([k, v]) => labels.push(<CopyableLabel key={`ml-${k}`} value={`${k}=${v}`} />));
        (term.labelSelector?.matchExpressions || []).forEach((expr: any, i: number) =>
            labels.push(<CopyableLabel key={`me-${i}`} value={formatMatchExpr(expr)} copyValue={expr.key} />));
        if (labels.length > 0)
            rules.push(
                <div key={`pref-${pi}`} className="flex flex-wrap items-center gap-1.5">
                    <span className="text-xs text-gray-500">preferred (w:{pref.weight}):</span>
                    {labels}
                </div>
            );
    });

    return rules;
}

function AffinityDisplay({ affinity }: { affinity: any }) {
    if (!affinity) return <span className="text-gray-500">None</span>;

    const sections: React.ReactNode[] = [];

    // Node Affinity
    if (affinity.nodeAffinity) {
        const na = affinity.nodeAffinity;
        const rules: React.ReactNode[] = [];

        na.requiredDuringSchedulingIgnoredDuringExecution?.nodeSelectorTerms?.forEach((term: any, ti: number) => {
            const exprs = [...(term.matchExpressions || []), ...(term.matchFields || [])];
            if (exprs.length > 0)
                rules.push(
                    <div key={`req-${ti}`} className="flex flex-wrap items-center gap-1.5">
                        <span className="text-xs text-gray-500">required:</span>
                        {exprs.map((expr: any, i: number) => (
                            <CopyableLabel key={i} value={formatMatchExpr(expr)} copyValue={expr.key} />
                        ))}
                    </div>
                );
        });

        na.preferredDuringSchedulingIgnoredDuringExecution?.forEach((pref: any, pi: number) => {
            const exprs = [...(pref.preference?.matchExpressions || []), ...(pref.preference?.matchFields || [])];
            if (exprs.length > 0)
                rules.push(
                    <div key={`pref-${pi}`} className="flex flex-wrap items-center gap-1.5">
                        <span className="text-xs text-gray-500">preferred (w:{pref.weight}):</span>
                        {exprs.map((expr: any, i: number) => (
                            <CopyableLabel key={i} value={formatMatchExpr(expr)} copyValue={expr.key} />
                        ))}
                    </div>
                );
        });

        if (rules.length > 0)
            sections.push(
                <div key="node">
                    <div className="text-xs text-blue-400 mb-1">Node</div>
                    <div className="space-y-1">{rules}</div>
                </div>
            );
    }

    // Pod Affinity
    if (affinity.podAffinity) {
        const rules = renderPodAffinityRules(affinity.podAffinity);
        if (rules.length > 0)
            sections.push(
                <div key="pod">
                    <div className="text-xs text-green-400 mb-1">Pod</div>
                    <div className="space-y-1">{rules}</div>
                </div>
            );
    }

    // Pod Anti-Affinity
    if (affinity.podAntiAffinity) {
        const rules = renderPodAffinityRules(affinity.podAntiAffinity);
        if (rules.length > 0)
            sections.push(
                <div key="anti">
                    <div className="text-xs text-amber-400 mb-1">Pod Anti-Affinity</div>
                    <div className="space-y-1">{rules}</div>
                </div>
            );
    }

    return sections.length > 0 ? (
        <div className="space-y-2.5">{sections}</div>
    ) : (
        <span className="text-gray-500">None</span>
    );
}

export default function PodInfoTab({ pod }: { pod: any }) {
    const { navigateWithSearch, openDiagnostic } = useUI();
    const { crds } = useK8s();

    const controller = getPodController(pod);
    const controllerViewId = controller ? getOwnerViewId(controller, crds) : null;
    const serviceAccount = pod.spec?.serviceAccountName || 'default';
    const labels = pod.metadata?.labels || {};
    const tolerations = pod.spec?.tolerations || [];
    const nodeSelector = pod.spec?.nodeSelector || {};
    const affinity = pod.spec?.affinity;
    const qosClass = pod.status?.qosClass || 'N/A';
    const podIPs = pod.status?.podIPs || (pod.status?.podIP ? [{ ip: pod.status.podIP }] : []);
    const nodeName = pod.spec?.nodeName;

    const handleControllerClick = useCallback(() => {
        if (!controller || !controllerViewId) return;
        navigateWithSearch(controllerViewId, `uid:"${controller.uid}"`);
    }, [controller, controllerViewId, navigateWithSearch]);

    const handleRBACCheck = useCallback(() => {
        openDiagnostic('rbac-checker', {
            initialSubject: {
                kind: 'ServiceAccount',
                name: serviceAccount,
                namespace: pod.metadata?.namespace
            }
        });
    }, [serviceAccount, pod.metadata?.namespace, openDiagnostic]);

    return (
        <div className="flex flex-col h-full overflow-auto p-4">
            <div className="bg-surface rounded-lg border border-border p-4">
                {/* Name */}
                <DetailRow label="Name" value={pod.metadata?.name} />

                {/* Namespace */}
                <DetailRow label="Namespace" value={pod.metadata?.namespace} />

                {/* Owner / Controlled By */}
                <DetailRow label="Owner">
                    {controller ? (
                        controllerViewId ? (
                            <button
                                onClick={handleControllerClick}
                                className="text-primary hover:text-primary/80 hover:underline transition-colors"
                                title={`Go to ${controller.kind}: ${controller.name}`}
                            >
                                {controller.kind}/{controller.name}
                            </button>
                        ) : (
                            <span className="text-gray-400">
                                {controller.kind}/{controller.name}
                            </span>
                        )
                    ) : (
                        <span className="text-gray-500">None</span>
                    )}
                </DetailRow>

                {/* Service Account */}
                <DetailRow label="Service Account">
                    <div className="flex items-center gap-2">
                        <CopyableLabel value={serviceAccount} />
                        <Tooltip content="Check RBAC">
                            <button
                                onClick={handleRBACCheck}
                                className="p-1 text-gray-400 hover:text-amber-400 hover:bg-white/10 rounded transition-colors"
                            >
                                <ShieldCheckIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                    </div>
                </DetailRow>

                {/* Created */}
                <DetailRow label="Created">
                    <span title={pod.metadata?.creationTimestamp}>
                        {formatAge(pod.metadata?.creationTimestamp)} ago
                    </span>
                </DetailRow>

                {/* Labels */}
                <DetailRow label="Labels">
                    {Object.keys(labels).length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(labels)
                                .sort(([a], [b]) => a.localeCompare(b))
                                .map(([key, value]) => (
                                    <CopyableLabel key={key} value={`${key}=${value}`} />
                                ))}
                        </div>
                    ) : (
                        <span className="text-gray-500">None</span>
                    )}
                </DetailRow>

                {/* Tolerations */}
                <DetailRow label="Tolerations">
                    {tolerations.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {tolerations.map((t: any, idx: number) => {
                                // Build display text
                                const parts = [];
                                if (t.key) parts.push(t.key);
                                if (t.operator && t.operator !== 'Equal') parts.push(t.operator);
                                if (t.value) parts.push(`=${t.value}`);
                                if (t.effect) parts.push(`:${t.effect}`);
                                if (t.tolerationSeconds !== undefined) parts.push(`(${t.tolerationSeconds}s)`);

                                const displayText = parts.length > 0 ? parts.join('') : 'Match all';
                                // Copy the key if available, otherwise the full text
                                const copyValue = t.key || displayText;

                                return (
                                    <CopyableLabel key={idx} value={displayText} copyValue={copyValue} />
                                );
                            })}
                        </div>
                    ) : (
                        <span className="text-gray-500">None</span>
                    )}
                </DetailRow>

                {/* Node Selector */}
                <DetailRow label="Node Selector">
                    {Object.keys(nodeSelector).length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(nodeSelector)
                                .sort(([a], [b]) => a.localeCompare(b))
                                .map(([key, value]) => (
                                    <CopyableLabel key={key} value={`${key}=${value}`} />
                                ))}
                        </div>
                    ) : (
                        <span className="text-gray-500">None</span>
                    )}
                </DetailRow>

                {/* Affinity */}
                <DetailRow label="Affinity">
                    <AffinityDisplay affinity={affinity} />
                </DetailRow>

                {/* QoS Class */}
                <DetailRow label="QoS Class">
                    <span className={`px-2 py-0.5 text-xs rounded border ${
                        qosClass === 'Guaranteed' ? 'bg-green-500/10 text-green-400 border-green-500/30' :
                        qosClass === 'Burstable' ? 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30' :
                        'bg-gray-500/10 text-gray-400 border-gray-500/30'
                    }`}>
                        {qosClass}
                    </span>
                </DetailRow>

                {/* Pod IPs */}
                <DetailRow label="Pod IPs">
                    {podIPs.length > 0 ? (
                        <div className="flex flex-wrap gap-2">
                            {podIPs.map((pip: any, idx: number) => (
                                <CopyableLabel key={idx} value={pip.ip} />
                            ))}
                        </div>
                    ) : (
                        <span className="text-gray-500">N/A</span>
                    )}
                </DetailRow>

                {/* Node Name */}
                <DetailRow label="Node">
                    {nodeName ? (
                        <CopyableLabel value={nodeName} />
                    ) : (
                        <span className="text-gray-500">N/A</span>
                    )}
                </DetailRow>
            </div>
        </div>
    );
}
