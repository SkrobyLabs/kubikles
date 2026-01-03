import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import { CronExpressionParser } from 'cron-parser';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import CronJobActionsMenu from './CronJobActionsMenu';
import { useCronJobs } from '../../../hooks/resources';
import { useCronJobActions } from './useCronJobActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteCronJob, GetCronJobYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

export default function CronJobList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    // Bulk action modal state
    const [bulkActionModal, setBulkActionModal] = useState({
        isOpen: false,
        action: null,
        items: [],
    });
    const [bulkProgress, setBulkProgress] = useState({
        current: 0,
        total: 0,
        status: 'idle',
        results: [],
    });

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
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                await DeleteCronJob(currentContext, namespace, name);
                results.push({ name, namespace, success: true, message: '' });
                Logger.info('CronJob deleted', { namespace, name });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
                Logger.error('Failed to delete cronjob', { namespace, name, error: err });
            }

            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }

        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, [currentContext]);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        Logger.info('Exporting YAML backup', { count: items.length });

        const entries = [];
        for (const item of items) {
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                const yaml = await GetCronJobYaml(namespace, name);
                entries.push({ namespace, name, kind: 'CronJob', yaml });
            } catch (err) {
                Logger.error('Failed to get YAML for backup', { namespace, name, error: err });
                entries.push({ namespace, name, kind: 'CronJob', yaml: `# Failed to fetch YAML: ${err}` });
            }
        }

        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        const defaultFilename = `cronjobs-backup-${timestamp}.zip`;

        try {
            await SaveYamlBackup(entries, defaultFilename);
            Logger.info('YAML backup saved');
        } catch (err) {
            Logger.error('Failed to save YAML backup', { error: err });
            if (err && err.toString() !== '') {
                alert('Failed to save backup: ' + err);
            }
        }
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
    const { cronJobs, loading } = useCronJobs(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleViewLogs, handleEditYaml, handleShowDependencies, handleRunNow, handleSuspend, handleDelete } = useCronJobActions();

    // Format duration for future time (reverse of formatAge)
    const formatDuration = (milliseconds) => {
        const seconds = Math.floor(milliseconds / 1000);
        if (seconds < 60) return `${seconds}s`;

        const minutes = Math.floor(seconds / 60);
        if (minutes < 60) return `${minutes}m`;

        const hours = Math.floor(minutes / 60);
        const remainingMinutes = minutes % 60;
        if (hours < 24) {
            return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
        }

        const days = Math.floor(hours / 24);
        const remainingHours = hours % 24;
        return remainingHours > 0 ? `${days}d ${remainingHours}h` : `${days}d`;
    };

    // Calculate next run time based on cron schedule
    const calculateNextRun = (cronJob) => {
        const isSuspended = cronJob.spec?.suspend || false;
        if (isSuspended) return 'Suspended';

        const schedule = cronJob.spec?.schedule;
        if (!schedule) return 'No schedule';

        try {
            // Parse the cron expression
            // Kubernetes uses standard 5-field cron format (minute hour day month weekday)
            const options = {
                currentDate: new Date()
            };

            const interval = CronExpressionParser.parse(schedule, options);

            // Get the next occurrence
            const nextRun = interval.next().toDate();
            const now = new Date();
            const diff = nextRun.getTime() - now.getTime();

            return formatDuration(diff);
        } catch (err) {
            console.error('Failed to parse cron schedule:', { schedule, error: err.message, stack: err.stack });
            return 'Invalid schedule';
        }
    };

    const formatLastRun = (lastScheduleTime) => {
        if (!lastScheduleTime) return 'Never';
        return formatAge(lastScheduleTime);
    };

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item) => item.metadata?.name,
            getValue: (item) => item.metadata?.name,
            initialSort: 'asc'
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (item) => item.metadata?.namespace,
            getValue: (item) => item.metadata?.namespace
        },
        {
            key: 'schedule',
            label: 'Schedule',
            render: (item) => (
                <span className="font-mono text-xs">{item.spec?.schedule || '-'}</span>
            ),
            getValue: (item) => item.spec?.schedule || ''
        },
        {
            key: 'suspend',
            label: 'Suspended',
            render: (item) => {
                const isSuspended = item.spec?.suspend || false;
                return isSuspended ? (
                    <CheckCircleIcon className="h-5 w-5 text-green-400" />
                ) : (
                    <span className="text-gray-500">-</span>
                );
            },
            getValue: (item) => item.spec?.suspend ? 'Yes' : 'No'
        },
        {
            key: 'lastRun',
            label: 'Last Run',
            render: (item) => formatLastRun(item.status?.lastScheduleTime),
            getValue: (item) => item.status?.lastScheduleTime || ''
        },
        {
            key: 'nextRun',
            label: 'Next Run',
            render: (item) => calculateNextRun(item),
            getValue: (item) => calculateNextRun(item)
        },
        {
            key: 'age',
            label: 'Age',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <CronJobActionsMenu
                    cronJob={item}
                    isOpen={activeMenuId === `cronjob-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `cronjob-${item.metadata.uid}`, buttonElement)}
                    onViewLogs={() => handleViewLogs(item)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onRunNow={() => handleRunNow(item)}
                    onSuspend={() => handleSuspend(item)}
                    onDelete={() => handleDelete(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleViewLogs, handleEditYaml, handleShowDependencies, handleRunNow, handleSuspend, handleDelete]);

    return (
        <>
            <ResourceList
                title="CronJobs"
                columns={columns}
                data={cronJobs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="cronjobs"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={handleBulkActionClose}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={handleBulkActionConfirm}
                onExportYaml={handleExportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
