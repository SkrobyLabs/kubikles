import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import { CronExpressionParser } from 'cron-parser';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import CronJobActionsMenu from './CronJobActionsMenu';
import { useCronJobs } from '../../../hooks/resources';
import { useCronJobActions } from './useCronJobActions';
import { useK8s } from '../../../context';
import { useMenu } from '../../../context';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteCronJob, GetCronJobYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function CronJobList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'CronJob',
        resourceType: 'cronjobs',
        isNamespaced: true,
        deleteApi: DeleteCronJob,
        getYamlApi: GetCronJobYaml,
        currentContext,
    });
    const { cronJobs, loading } = useCronJobs(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleViewLogs, handleEditYaml, handleShowDependencies, handleRunNow, handleSuspend } = useCronJobActions();

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
        // Hidden by default columns
        {
            key: 'activeJobs',
            label: 'Active Jobs',
            defaultHidden: true,
            render: (item) => (item.status?.active || []).length,
            getValue: (item) => (item.status?.active || []).length,
        },
        {
            key: 'concurrencyPolicy',
            label: 'Concurrency',
            defaultHidden: true,
            render: (item) => item.spec?.concurrencyPolicy || 'Allow',
            getValue: (item) => item.spec?.concurrencyPolicy || 'Allow',
        },
        {
            key: 'successfulJobsLimit',
            label: 'Keep Success',
            defaultHidden: true,
            render: (item) => item.spec?.successfulJobsHistoryLimit ?? 3,
            getValue: (item) => item.spec?.successfulJobsHistoryLimit ?? 3,
        },
        {
            key: 'failedJobsLimit',
            label: 'Keep Failed',
            defaultHidden: true,
            render: (item) => item.spec?.failedJobsHistoryLimit ?? 1,
            getValue: (item) => item.spec?.failedJobsHistoryLimit ?? 1,
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (item) => {
                const containers = item.spec?.jobTemplate?.spec?.template?.spec?.containers || [];
                if (containers.length === 0) return '-';
                if (containers.length === 1) return <span title={containers[0].image}>{containers[0].image?.split('/').pop()}</span>;
                return <span title={containers.map(c => c.image).join('\n')}>{containers.length} images</span>;
            },
            getValue: (item) => item.spec?.jobTemplate?.spec?.template?.spec?.containers?.[0]?.image || '',
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
                    onDelete={() => openBulkDelete([item])}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleViewLogs, handleEditYaml, handleShowDependencies, handleRunNow, handleSuspend, openBulkDelete]);

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
