import React, { useMemo, useState, useCallback, useEffect } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import { useJobs } from '../../../hooks/resources';
import { useJobActions } from './useJobActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteJob, GetJobYaml } from '../../../../wailsjs/go/main/App';
import JobActionsMenu from './JobActionsMenu';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import { formatAge } from '../../../utils/formatting';
import { getOwnerViewId } from '../../../utils/owner-navigation';
import { getJobConditionColor } from '../../../utils/k8s-helpers';

// Get controller from owner references
function getController(item) {
    const owners = item.metadata?.ownerReferences || [];
    const controller = owners.find(owner => owner.controller);
    return controller ? { kind: controller.kind, name: controller.name, uid: controller.uid, apiVersion: controller.apiVersion } : null;
}

export default function JobList({ isVisible }) {
    const { currentContext, selectedNamespaces, namespaces, setSelectedNamespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for owner reference resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const { activeMenuId, setActiveMenuId } = useMenu();
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
        resourceLabel: 'Job',
        resourceType: 'jobs',
        isNamespaced: true,
        deleteApi: DeleteJob,
        getYamlApi: GetJobYaml,
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
    const { jobs, loading } = useJobs(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useJobActions();

    const getCompletions = (job) => {
        const succeeded = job.status?.succeeded || 0;
        const completions = job.spec?.completions || '?';
        return `${succeeded}/${completions}`;
    };

    const getCondition = (job) => {
        const conditions = job.status?.conditions || [];
        if (conditions.length === 0) return '-';
        const lastCondition = conditions[conditions.length - 1];
        return lastCondition.type || '-';
    };

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            width: '25%',
            render: (job) => job.metadata.name
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (job) => job.metadata?.namespace
        },
        {
            key: 'completions',
            label: 'Completions',
            width: '20%',
            render: (job) => getCompletions(job)
        },
        {
            key: 'condition',
            label: 'Condition',
            width: '20%',
            render: (job) => {
                const condition = getCondition(job);
                return <span className={getJobConditionColor(condition)}>{condition}</span>;
            }
        },
        {
            key: 'age',
            label: 'Age',
            render: (job) => formatAge(job.metadata?.creationTimestamp),
            getValue: (job) => job.metadata?.creationTimestamp
        },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item) => {
                const controller = getController(item);
                if (!controller) {
                    return <span className="text-gray-600">-</span>;
                }

                const viewId = getOwnerViewId(controller, crds);

                if (viewId) {
                    return (
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                navigateWithSearch(viewId, `uid:"${controller.uid}"`);
                            }}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors"
                            title={`Go to ${controller.kind}: ${controller.name}`}
                        >
                            {controller.kind}
                        </button>
                    );
                }

                return (
                    <span className="text-gray-400" title={controller.name}>
                        {controller.kind}
                    </span>
                );
            },
            getValue: (item) => getController(item)?.kind || ''
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (job) => (
                <JobActionsMenu
                    job={job}
                    isOpen={activeMenuId === `job-${job.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `job-${job.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(job) => openBulkDelete([job])}
                    onViewLogs={handleViewLogs}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkDelete, handleViewLogs, navigateWithSearch, crds]);

    return (
        <>
            <ResourceList
                title="Jobs"
                columns={columns}
                data={jobs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="jobs"
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
