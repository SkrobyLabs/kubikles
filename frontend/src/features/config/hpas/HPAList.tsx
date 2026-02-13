import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import HPAActionsMenu from './HPAActionsMenu';
import { useHPAs } from '~/hooks/resources';
import { useHPAActions } from './useHPAActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteHPA, GetHPAYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function HPAList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { hpas, loading } = useHPAs(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useHPAActions();
    const selection = useSelection();

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'HorizontalPodAutoscaler',
        resourceType: 'hpas',
        isNamespaced: true,
        deleteApi: DeleteHPA,
        getYamlApi: GetHPAYaml,

    });

    const getScaleTarget = (hpa: any) => {
        const ref = hpa.spec?.scaleTargetRef;
        if (!ref) return '-';
        return `${ref.kind}/${ref.name}`;
    };

    const getMinMax = (hpa: any) => {
        const min = hpa.spec?.minReplicas ?? 1;
        const max = hpa.spec?.maxReplicas ?? '-';
        return `${min}/${max}`;
    };

    const getReplicas = (hpa: any) => {
        return hpa.status?.currentReplicas ?? '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        { key: 'reference', label: 'Reference', render: (item: any) => getScaleTarget(item), getValue: (item: any) => getScaleTarget(item) },
        { key: 'minmax', label: 'Min/Max', render: (item: any) => getMinMax(item), getValue: (item: any) => getMinMax(item) },
        { key: 'replicas', label: 'Replicas', render: (item: any) => getReplicas(item), getValue: (item: any) => getReplicas(item) },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <HPAActionsMenu
                    hpa={item}
                    isOpen={activeMenuId === `hpa-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `hpa-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(item: any) => openBulkDelete([item])}
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
                title="Horizontal Pod Autoscalers"
                columns={columns}
                data={hpas}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="hpas"
                onRowClick={handleShowDetails}
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
