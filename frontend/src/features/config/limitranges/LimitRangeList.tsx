import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import LimitRangeActionsMenu from './LimitRangeActionsMenu';
import { useLimitRanges } from '~/hooks/resources';
import { useLimitRangeActions } from './useLimitRangeActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteLimitRange, GetLimitRangeYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { limitRangeSemantics, limitRangeCount } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildLimitRangeViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const LIMITRANGE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const LIMITRANGE_VIEW_OPTIONS = { sort: LIMITRANGE_INITIAL_SORT, columns: defaultViewColumns('limitranges') };
const NO_DATA: any[] = [];

export default function LimitRangeList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { limitRanges, loading, loadState, view } = useLimitRanges(currentContext, selectedNamespaces, isVisible, LIMITRANGE_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useLimitRangeActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'LimitRange',
        resourceType: 'limitranges',
        isNamespaced: true,
        deleteApi: DeleteLimitRange,
        getYamlApi: GetLimitRangeYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: limitRangeSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: limitRangeSemantics.namespace.getValue },
        { key: 'limits', label: 'Limits', render: (item: any) => limitRangeCount(item), getValue: limitRangeSemantics.limits.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: limitRangeSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={LimitRangeActionsMenu}
                    menuId={`limitrange-${item.metadata.uid}`}
                    limitRange={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(item: any) => openBulkDelete([item])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildLimitRangeViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Limit Ranges"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : limitRanges}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={LIMITRANGE_INITIAL_SORT}
                resourceType="limitranges"
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
