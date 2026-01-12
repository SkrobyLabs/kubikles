import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import MutatingWebhookActionsMenu from './MutatingWebhookActionsMenu';
import { useMutatingWebhookConfigurations } from '../../../hooks/resources';
import { useMutatingWebhookActions } from './useMutatingWebhookActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteMutatingWebhookConfiguration, GetMutatingWebhookConfigurationYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

export default function MutatingWebhookList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useMenu();
    const { mutatingWebhookConfigurations, loading } = useMutatingWebhookConfigurations(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = useMutatingWebhookActions();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    const [bulkActionModal, setBulkActionModal] = useState({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState({ current: 0, total: 0, status: 'idle', results: [] });

    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkActionConfirm = useCallback(async (items) => {
        Logger.info('Bulk delete started', { count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));
        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const name = item.metadata?.name;
            try {
                await DeleteMutatingWebhookConfiguration(name);
                results.push({ name, namespace: '', success: true, message: '' });
            } catch (err) {
                results.push({ name, namespace: '', success: false, message: err.toString() });
            }
            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }
        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, []);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        const entries = [];
        for (const item of items) {
            try {
                const yaml = await GetMutatingWebhookConfigurationYaml(item.metadata?.name);
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'MutatingWebhookConfiguration', yaml });
            } catch (err) {
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'MutatingWebhookConfiguration', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `mutatingwebhookconfigurations-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
    }, []);

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

    const getWebhookCount = (config) => {
        return (config.webhooks || []).length;
    };

    const getFailurePolicy = (config) => {
        const policies = new Set();
        (config.webhooks || []).forEach(wh => {
            policies.add(wh.failurePolicy || 'Fail');
        });
        return Array.from(policies).join(', ') || '-';
    };

    const getReinvocationPolicy = (config) => {
        const policies = new Set();
        (config.webhooks || []).forEach(wh => {
            policies.add(wh.reinvocationPolicy || 'Never');
        });
        return Array.from(policies).join(', ') || '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'webhooks', label: 'Webhooks', render: (item) => getWebhookCount(item), getValue: (item) => getWebhookCount(item) },
        { key: 'failurePolicy', label: 'Failure Policy', render: (item) => getFailurePolicy(item), getValue: (item) => getFailurePolicy(item) },
        { key: 'reinvocation', label: 'Reinvocation', render: (item) => getReinvocationPolicy(item), getValue: (item) => getReinvocationPolicy(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <MutatingWebhookActionsMenu
                    webhook={item}
                    isOpen={activeMenuId === `mutatingwebhook-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `mutatingwebhook-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleDelete]);

    return (
        <>
            <ResourceList
                title="Mutating Webhook Configurations"
                columns={columns}
                data={mutatingWebhookConfigurations}
                isLoading={loading}
                showNamespaceSelector={false}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="mutatingwebhookconfigurations"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />
        </>
    );
}
