import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useNodes } from '../../../hooks/useNodes';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import NodeActionsMenu from './NodeActionsMenu';
import { useNodeActions } from './useNodeActions';

// Helper to get node conditions summary
const getConditionsSummary = (node) => {
    const conditions = node.status?.conditions || [];
    const isUnschedulable = node.spec?.unschedulable === true;

    const issues = [];

    // Check Ready condition
    const readyCondition = conditions.find(c => c.type === 'Ready');
    const isReady = readyCondition?.status === 'True';

    if (!isReady) {
        issues.push({ type: 'NotReady', severity: 'error' });
    }

    if (isUnschedulable) {
        issues.push({ type: 'SchedulingDisabled', severity: 'warning' });
    }

    // Check pressure conditions (these are problems when True)
    const pressureConditions = ['MemoryPressure', 'DiskPressure', 'PIDPressure'];
    pressureConditions.forEach(condType => {
        const cond = conditions.find(c => c.type === condType);
        if (cond?.status === 'True') {
            issues.push({ type: condType, severity: 'error' });
        }
    });

    // Check NetworkUnavailable (problem when True)
    const networkCond = conditions.find(c => c.type === 'NetworkUnavailable');
    if (networkCond?.status === 'True') {
        issues.push({ type: 'NetworkUnavailable', severity: 'error' });
    }

    return { isReady, isUnschedulable, issues };
};

// Conditions cell component
const ConditionsCell = ({ node }) => {
    const { isReady, issues } = getConditionsSummary(node);

    if (issues.length === 0) {
        return <span className="text-green-400">Ready</span>;
    }

    return (
        <div className="flex flex-wrap gap-1">
            {issues.map((issue, idx) => (
                <span
                    key={idx}
                    className={`px-1.5 py-0.5 text-xs rounded ${
                        issue.severity === 'error'
                            ? 'bg-red-500/20 text-red-400'
                            : 'bg-yellow-500/20 text-yellow-400'
                    }`}
                >
                    {issue.type}
                </span>
            ))}
        </div>
    );
};

// Taints cell component with tooltip
const TaintsCell = ({ node }) => {
    const taints = node.spec?.taints || [];
    const count = taints.length;

    if (count === 0) {
        return <span className="text-gray-500">-</span>;
    }

    const tooltipLines = taints.map(t =>
        `${t.key}${t.value ? '=' + t.value : ''}:${t.effect}`
    );

    return (
        <span className="relative group cursor-help text-gray-300 underline decoration-dotted">
            {count}
            <div className="absolute z-50 invisible opacity-0 group-hover:visible group-hover:opacity-100 transition-opacity delay-500 bottom-full left-1/2 -translate-x-1/2 mb-1 px-2 py-1 text-xs bg-[#1e1e1e] border border-[#3d3d3d] rounded shadow-lg whitespace-pre text-left">
                {tooltipLines.map((line, idx) => (
                    <div key={idx}>{line}</div>
                ))}
            </div>
        </span>
    );
};

export default function NodeList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { nodes, loading, refetch } = useNodes(currentContext, isVisible);
    const { handleEditYaml, handleCordonUncordon, handleShell, handleDelete } = useNodeActions(refetch);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'conditions',
            label: 'Conditions',
            render: (item) => <ConditionsCell node={item} />,
            getValue: (item) => {
                const { issues } = getConditionsSummary(item);
                return issues.length === 0 ? 'Ready' : issues.map(i => i.type).join(',');
            }
        },
        {
            key: 'taints',
            label: 'Taints',
            align: 'center',
            render: (item) => <TaintsCell node={item} />,
            getValue: (item) => (item.spec?.taints || []).length
        },
        { key: 'version', label: 'Version', render: (item) => item.status?.nodeInfo?.kubeletVersion, getValue: (item) => item.status?.nodeInfo?.kubeletVersion },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <NodeActionsMenu
                    node={item}
                    isOpen={activeMenuId === `node-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `node-${item.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onCordonUncordon={handleCordonUncordon}
                    onShell={handleShell}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleCordonUncordon, handleShell, handleDelete]);

    return (
        <ResourceList
            title="Nodes"
            columns={columns}
            data={nodes}
            isLoading={loading}
            showNamespaceSelector={false}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="nodes"
        />
    );
}
