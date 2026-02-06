import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import PriorityClassActionsMenu from './PriorityClassActionsMenu';
import { usePriorityClasses } from '~/hooks/resources';
import { usePriorityClassActions } from './usePriorityClassActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeletePriorityClass, GetPriorityClassYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function PriorityClassList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { priorityClasses, loading } = usePriorityClasses(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = usePriorityClassActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'PriorityClass',
        resourceType: 'priorityclasses',
        isNamespaced: false,
        deleteApi: DeletePriorityClass,
        getYamlApi: GetPriorityClassYaml,

    });

    const formatValue = (value) => {
        if (value >= 1000000000) return `${(value / 1000000000).toFixed(1)}B`;
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
        return value?.toString() || '0';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'value',
            label: 'Value',
            render: (item) => (
                <span className="font-mono">{formatValue(item.value)}</span>
            ),
            getValue: (item) => item.value || 0
        },
        {
            key: 'globalDefault',
            label: 'Global Default',
            render: (item) => (
                <span className={item.globalDefault ? 'text-green-400' : 'text-gray-500'}>
                    {item.globalDefault ? 'Yes' : 'No'}
                </span>
            ),
            getValue: (item) => item.globalDefault ? 'Yes' : 'No'
        },
        {
            key: 'preemption',
            label: 'Preemption',
            render: (item) => item.preemptionPolicy || 'PreemptLowerPriority',
            getValue: (item) => item.preemptionPolicy || 'PreemptLowerPriority'
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PriorityClassActionsMenu
                    priorityClass={item}
                    isOpen={activeMenuId === `priorityclass-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `priorityclass-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(priorityClass) => openBulkDelete([priorityClass])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList
                title="Priority Classes"
                columns={columns}
                data={priorityClasses}
                isLoading={loading}
                showNamespaceSelector={false}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'value', direction: 'desc' }}
                resourceType="priorityclasses"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
