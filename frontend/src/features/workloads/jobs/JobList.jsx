import React from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useJobs } from '../../../hooks/useJobs';
import { useJobActions } from './useJobActions';
import { useK8s } from '../../../context/K8sContext';
import JobActionsMenu from './JobActionsMenu';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';

export default function JobList({ isVisible }) {
    const { currentContext, currentNamespace, namespaces, setCurrentNamespace } = useK8s();
    const { jobs, loading } = useJobs(currentContext, currentNamespace, isVisible);
    const { handleEditYaml, handleDelete, handleViewLogs } = useJobActions(currentNamespace);
    const [activeMenuId, setActiveMenuId] = React.useState(null);

    const getCompletions = (job) => {
        const succeeded = job.status?.succeeded || 0;
        const completions = job.spec?.completions || '?';
        return `${succeeded}/${completions}`;
    };

    const getCondition = (job) => {
        const conditions = job.status?.conditions || [];
        if (conditions.length === 0) return '-';

        // Get the last condition
        const lastCondition = conditions[conditions.length - 1];
        return lastCondition.type || '-';
    };

    const getAge = (creationTimestamp) => {
        if (!creationTimestamp) return '-';
        const created = new Date(creationTimestamp);
        const now = new Date();
        const diffMs = now - created;
        const diffMins = Math.floor(diffMs / 60000);
        const diffHours = Math.floor(diffMins / 60);
        const diffDays = Math.floor(diffHours / 24);

        if (diffDays > 0) return `${diffDays}d`;
        if (diffHours > 0) return `${diffHours}h`;
        if (diffMins > 0) return `${diffMins}m`;
        return '<1m';
    };

    const columns = [
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
            render: (job) => getCondition(job)
        },
        {
            key: 'age',
            label: 'Age',
            width: '20%',
            render: (job) => getAge(job.metadata.creationTimestamp)
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            width: '15%',
            render: (job) => (
                <JobActionsMenu
                    job={job}
                    isOpen={activeMenuId === `job-${job.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `job-${job.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                    onViewLogs={handleViewLogs}
                />
            )
        },
    ];

    return (
        <ResourceList
            title="Jobs"
            columns={columns}
            data={jobs}
            loading={loading}
            emptyMessage="No jobs found in this namespace"
            namespaces={namespaces}
            currentNamespace={currentNamespace}
            onNamespaceChange={setCurrentNamespace}
        />
    );
}
