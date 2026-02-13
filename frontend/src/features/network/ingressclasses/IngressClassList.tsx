import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import IngressClassActionsMenu from './IngressClassActionsMenu';
import { useIngressClasses } from '~/hooks/resources';
import { useIngressClassActions } from './useIngressClassActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteIngressClass, GetIngressClassYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function IngressClassList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { ingressClasses, loading } = useIngressClasses(currentContext, isVisible) as any;
    const { handleEditYaml } = useIngressClassActions();
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

    const isDefault = (ingressClass: any) => {
        return ingressClass.metadata?.annotations?.['ingressclass.kubernetes.io/is-default-class'] === 'true';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'controller', label: 'Controller', render: (item: any) => item.spec?.controller || '-', getValue: (item: any) => item.spec?.controller || '' },
        {
            key: 'default',
            label: 'Default',
            render: (item: any) => isDefault(item) ? (
                <CheckCircleIcon className="h-5 w-5 text-green-400" />
            ) : (
                <span className="text-gray-500">-</span>
            ),
            getValue: (item: any) => isDefault(item) ? 'Yes' : 'No'
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <IngressClassActionsMenu
                    ingressClass={item}
                    isOpen={activeMenuId === `ingressclass-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `ingressclass-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(ingressClass: any) => openBulkDelete([ingressClass])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList
                title="Ingress Classes"
                columns={columns}
                data={ingressClasses}
                isLoading={loading}
                showNamespaceSelector={false}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
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
