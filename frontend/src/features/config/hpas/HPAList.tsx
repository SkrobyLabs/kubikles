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

export default function HPAList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { hpas, loading } = useHPAs(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useHPAActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'HorizontalPodAutoscaler',
        resourceType: 'hpas',
        isNamespaced: true,
        deleteApi: DeleteHPA,
        getYamlApi: GetHPAYaml,

    });

    const getScaleTarget = (hpa) => {
        const ref = hpa.spec?.scaleTargetRef;
        if (!ref) return '-';
        return `${ref.kind}/${ref.name}`;
    };

    const getMinMax = (hpa) => {
        const min = hpa.spec?.minReplicas ?? 1;
        const max = hpa.spec?.maxReplicas ?? '-';
        return `${min}/${max}`;
    };

    const getReplicas = (hpa) => {
        return hpa.status?.currentReplicas ?? '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'reference', label: 'Reference', render: (item) => getScaleTarget(item), getValue: (item) => getScaleTarget(item) },
        { key: 'minmax', label: 'Min/Max', render: (item) => getMinMax(item), getValue: (item) => getMinMax(item) },
        { key: 'replicas', label: 'Replicas', render: (item) => getReplicas(item), getValue: (item) => getReplicas(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <HPAActionsMenu
                    hpa={item}
                    isOpen={activeMenuId === `hpa-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `hpa-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(item) => openBulkDelete([item])}
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
