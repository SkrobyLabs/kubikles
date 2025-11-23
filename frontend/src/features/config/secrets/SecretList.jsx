import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useSecrets } from '../../../hooks/useSecrets';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import SecretActionsMenu from './SecretActionsMenu';
import { useSecretActions } from './useSecretActions';

export default function SecretList({ isVisible }) {
    const { currentContext, currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { secrets, loading } = useSecrets(currentContext, currentNamespace, isVisible);
    const { handleEditYaml, handleDelete } = useSecretActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'type', label: 'Type', render: (item) => item.type, getValue: (item) => item.type },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <SecretActionsMenu
                    secret={item}
                    isOpen={activeMenuId === `secret-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `secret-${item.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleDelete]);

    return (
        <ResourceList
            title="Secrets"
            columns={columns}
            data={secrets}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={currentNamespace}
            onNamespaceChange={setCurrentNamespace}
            showNamespaceSelector={true}
        />
    );
}
