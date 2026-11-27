import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import IngressClassActionsMenu from './IngressClassActionsMenu';
import { useIngressClasses } from '~/hooks/resources';
import { useIngressClassActions } from './useIngressClassActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteIngressClass, GetIngressClassYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { ingressClassSemantics, isDefaultIngressClass } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildIngressClassViewColumns } from './viewColumns';

const INGRESSCLASS_INITIAL_SORT = { key: 'age', direction: 'desc' };
const INGRESSCLASS_VIEW_OPTIONS = { sort: INGRESSCLASS_INITIAL_SORT, columns: defaultViewColumns('ingressclasses') };
const NO_DATA: any[] = [];

export default function IngressClassList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { ingressClasses, loading, loadState, view } = useIngressClasses(currentContext, isVisible, INGRESSCLASS_VIEW_OPTIONS) as any;
    const { handleEditYaml } = useStableCallbacks(useIngressClassActions());
    const selection = useSelection();

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'IngressClass',
        resourceType: 'ingressclasses',
        isNamespaced: false,
        deleteApi: DeleteIngressClass,
        getYamlApi: GetIngressClassYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: ingressClassSemantics.name.getValue },
        { key: 'controller', label: 'Controller', render: (item: any) => item.spec?.controller || '-', getValue: ingressClassSemantics.controller.getValue },
        {
            key: 'default',
            label: 'Default',
            render: (item: any) => isDefaultIngressClass(item) ? (
                <CheckCircleIcon className="h-5 w-5 text-green-400" />
            ) : (
                <span className="text-gray-500">-</span>
            ),
            getValue: ingressClassSemantics.default.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: ingressClassSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={IngressClassActionsMenu}
                    menuId={`ingressclass-${item.metadata.uid}`}
                    ingressClass={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(ingressClass: any) => openBulkDelete([ingressClass])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildIngressClassViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Ingress Classes"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : ingressClasses}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={INGRESSCLASS_INITIAL_SORT}
                resourceType="ingressclasses"
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
