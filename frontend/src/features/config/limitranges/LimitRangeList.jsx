import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import LimitRangeActionsMenu from './LimitRangeActionsMenu';
import { useLimitRanges } from '../../../hooks/resources';
import { useLimitRangeActions } from './useLimitRangeActions';
import { useK8s } from '../../../context/K8sContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteLimitRange, GetLimitRangeYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';

export default function LimitRangeList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useMenu();
    const { limitRanges, loading } = useLimitRanges(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useLimitRangeActions();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'LimitRange',
        resourceType: 'limitranges',
        isNamespaced: true,
        deleteApi: DeleteLimitRange,
        getYamlApi: GetLimitRangeYaml,
        currentContext,
    });

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            setMenuPosition({
                top: rect.bottom + 4,
                left: rect.right - 192
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId]);

    const getLimitCount = (lr) => {
        return lr.spec?.limits?.length ?? 0;
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'limits', label: 'Limits', render: (item) => getLimitCount(item), getValue: (item) => getLimitCount(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <LimitRangeActionsMenu
                    limitRange={item}
                    isOpen={activeMenuId === `limitrange-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `limitrange-${item.metadata.uid}`, buttonElement)}
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
                title="Limit Ranges"
                columns={columns}
                data={limitRanges}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="limitranges"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
