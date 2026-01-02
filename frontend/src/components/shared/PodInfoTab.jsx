import React, { useCallback } from 'react';
import { useUI } from '../../context/UIContext';
import { getPodController } from '../../utils/k8s-helpers';
import { formatAge } from '../../utils/formatting';
import { CopyableLabel, DetailRow } from './DetailComponents';

// Map controller kind to view name
const kindToView = {
    'ReplicaSet': 'replicasets',
    'Deployment': 'deployments',
    'StatefulSet': 'statefulsets',
    'DaemonSet': 'daemonsets',
    'Job': 'jobs',
    'CronJob': 'cronjobs',
};

export default function PodInfoTab({ pod }) {
    const { navigateWithSearch } = useUI();

    const controller = getPodController(pod);
    const labels = pod.metadata?.labels || {};
    const tolerations = pod.spec?.tolerations || [];
    const qosClass = pod.status?.qosClass || 'N/A';
    const podIPs = pod.status?.podIPs || (pod.status?.podIP ? [{ ip: pod.status.podIP }] : []);
    const nodeName = pod.spec?.nodeName;

    const handleControllerClick = useCallback(() => {
        if (!controller) return;
        const viewName = kindToView[controller.kind];
        if (viewName) {
            navigateWithSearch(viewName, `uid:"${controller.uid}"`);
        }
    }, [controller, navigateWithSearch]);

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
                        kindToView[controller.kind] ? (
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
                            {tolerations.map((t, idx) => {
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
                            {podIPs.map((pip, idx) => (
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
