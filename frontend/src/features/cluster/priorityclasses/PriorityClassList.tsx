import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import PriorityClassActionsMenu from './PriorityClassActionsMenu';
import { usePriorityClasses } from '~/hooks/resources';
import { usePriorityClassActions } from './usePriorityClassActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeletePriorityClass, GetPriorityClassYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { priorityClassSemantics } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildPriorityClassViewColumns, formatPriorityValue } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const PRIORITYCLASS_INITIAL_SORT = { key: 'value', direction: 'desc' };
const PRIORITYCLASS_VIEW_OPTIONS = { sort: PRIORITYCLASS_INITIAL_SORT, columns: defaultViewColumns('priorityclasses') };
const NO_DATA: any[] = [];

export default function PriorityClassList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { priorityClasses, loading, loadState, view } = usePriorityClasses(currentContext, isVisible, PRIORITYCLASS_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(usePriorityClassActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'PriorityClass',
        resourceType: 'priorityclasses',
        isNamespaced: false,
        deleteApi: DeletePriorityClass,
        getYamlApi: GetPriorityClassYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: priorityClassSemantics.name.getValue },
        {
            key: 'value',
            label: 'Value',
            render: (item: any) => (
                <span className="font-mono">{formatPriorityValue(item.value)}</span>
            ),
            getValue: priorityClassSemantics.value.getValue
        },
        {
            key: 'globalDefault',
            label: 'Global Default',
            render: (item: any) => (
                <span className={item.globalDefault ? 'text-green-400' : 'text-gray-500'}>
                    {item.globalDefault ? 'Yes' : 'No'}
                </span>
            ),
            getValue: priorityClassSemantics.globalDefault.getValue
        },
        {
            key: 'preemption',
            label: 'Preemption',
            render: (item: any) => item.preemptionPolicy || 'PreemptLowerPriority',
            getValue: priorityClassSemantics.preemption.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: priorityClassSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={PriorityClassActionsMenu}
                    menuId={`priorityclass-${item.metadata.uid}`}
                    priorityClass={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(priorityClass: any) => openBulkDelete([priorityClass])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildPriorityClassViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Priority Classes"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : priorityClasses}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={PRIORITYCLASS_INITIAL_SORT}
                resourceType="priorityclasses"
                onRowClick={view ? showDetails : handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </>
    );
}
