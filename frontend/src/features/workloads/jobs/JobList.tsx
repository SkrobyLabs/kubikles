import React, { useMemo, useState, useCallback, useEffect } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useJobs } from '~/hooks/resources';
import { useJobActions } from './useJobActions';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useMenu } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteJob, GetJobYaml } from 'wailsjs/go/main/App';
import JobActionsMenu from './JobActionsMenu';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import { formatAge } from '~/utils/formatting';
import { getJobConditionColor } from '~/utils/k8s-helpers';

import { jobSemantics, jobCompletions, jobCondition, formatJobDuration } from './semantics';
import { getController } from '../workloadSemantics';
import { renderControlledBy } from '../ControlledBy';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildJobViewColumns, JOB_HIDDEN_BY_DEFAULT, JOB_PINNED_COLUMNS, JOB_CARRIER_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const JOB_INITIAL_SORT = { key: 'age', direction: 'desc' };
const JOB_VIEW_OPTIONS = { sort: JOB_INITIAL_SORT, columns: defaultViewColumns('jobs', JOB_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function JobList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, namespaces, setSelectedNamespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for owner reference resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const selection = useSelection();

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Job',
        resourceType: 'jobs',
        isNamespaced: true,
        deleteApi: DeleteJob,
        getYamlApi: GetJobYaml,

    });
    const { jobs, loading, loadState, view } = useJobs(currentContext, selectedNamespaces, isVisible, JOB_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useStableCallbacks(useJobActions());

    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            width: '25%',
            render: (job: any) => job.metadata.name
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (job: any) => job.metadata?.namespace
        },
        {
            key: 'completions',
            label: 'Completions',
            width: '20%',
            render: (job: any) => jobCompletions(job)
        },
        {
            key: 'condition',
            label: 'Condition',
            width: '20%',
            render: (job: any) => {
                const condition = jobCondition(job);
                return <span className={getJobConditionColor(condition)}>{condition}</span>;
            }
        },
        {
            key: 'age',
            label: 'Age',
            render: (job: any) => formatAge(job.metadata?.creationTimestamp),
            getValue: jobSemantics.age.getValue
        },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item: any) => renderControlledBy(getController(item), crds, navigateWithSearch),
            getValue: jobSemantics.controlledBy.getValue
        },
        // Hidden by default columns
        {
            key: 'parallelism',
            label: 'Parallelism',
            defaultHidden: true,
            render: (job: any) => job.spec?.parallelism ?? 1,
            getValue: jobSemantics.parallelism.getValue,
        },
        {
            key: 'backoffLimit',
            label: 'Backoff Limit',
            defaultHidden: true,
            render: (job: any) => job.spec?.backoffLimit ?? 6,
            getValue: jobSemantics.backoffLimit.getValue,
        },
        {
            key: 'activeDeadline',
            label: 'Deadline (s)',
            defaultHidden: true,
            render: (job: any) => job.spec?.activeDeadlineSeconds ?? <span className="text-gray-500">-</span>,
            getValue: jobSemantics.activeDeadline.getValue,
        },
        {
            key: 'startTime',
            label: 'Start Time',
            defaultHidden: true,
            render: (job: any) => job.status?.startTime ? formatAge(job.status.startTime) + ' ago' : <span className="text-gray-500">-</span>,
            getValue: jobSemantics.startTime.getValue,
        },
        {
            key: 'duration',
            label: 'Duration',
            defaultHidden: true,
            render: (job: any) => formatJobDuration(job.status?.startTime, job.status?.completionTime) ?? <span className="text-gray-500">-</span>,
            getValue: jobSemantics.duration.getValue,
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (job: any) => {
                const containers = job.spec?.template?.spec?.containers || [];
                if (containers.length === 0) return '-';
                if (containers.length === 1) return <span title={containers[0].image}>{containers[0].image?.split('/').pop()}</span>;
                return <span title={containers.map((c: any) => c.image).join('\n')}>{containers.length} images</span>;
            },
            getValue: jobSemantics.image.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (job: any) => (
                <MenuBound
                    as={JobActionsMenu}
                    menuId={`job-${job.metadata.uid}`}
                    job={job}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(job: any) => openBulkDelete([job])}
                    onViewLogs={handleViewLogs}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleEditYaml, handleShowDependencies, openBulkDelete, handleViewLogs, navigateWithSearch, crds]);

    const viewColumns = useMemo(
        () => buildJobViewColumns({
            crds,
            navigateWithSearch,
            onEditYaml: handleEditYaml as (item: any) => void,
            onShowDependencies: handleShowDependencies as (item: any) => void,
            onDelete: item => openBulkDelete([item]),
            onViewLogs: handleViewLogs as (item: any) => void,
        }),
        [crds, navigateWithSearch, handleEditYaml, handleShowDependencies, openBulkDelete, handleViewLogs]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Jobs"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={JOB_PINNED_COLUMNS}
                viewCarrierColumns={JOB_CARRIER_COLUMNS}
                data={view ? NO_DATA : jobs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={JOB_INITIAL_SORT}
                resourceType="jobs"
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
