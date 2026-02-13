import React, { useMemo, useState } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useSecrets } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteSecret, GetSecretYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import SecretActionsMenu from './SecretActionsMenu';
import { useSecretActions } from './useSecretActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';

const HELM_RELEASE_SECRET_TYPE = 'helm.sh/release.v1';

export default function SecretList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { secrets, loading } = useSecrets(currentContext, selectedNamespaces, isVisible) as any;
    const [hideHelmSecrets, setHideHelmSecrets] = useState(true);

    // Filter out Helm release secrets if toggle is enabled
    const filteredSecrets = useMemo(() => {
        if (!hideHelmSecrets) return secrets;
        return secrets.filter((secret: any) => secret.type !== HELM_RELEASE_SECRET_TYPE);
    }, [secrets, hideHelmSecrets]);
    const { handleEditYaml, handleEditKeyValue, handleShowDependencies } = useSecretActions();
    const selection = useSelection();

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Secret',
        resourceType: 'secrets',
        isNamespaced: true,
        deleteApi: DeleteSecret,
        getYamlApi: GetSecretYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        { key: 'type', label: 'Type', render: (item: any) => item.type, getValue: (item: any) => item.type },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        // Hidden by default columns
        {
            key: 'keys',
            label: 'Keys',
            defaultHidden: true,
            render: (item: any) => {
                const keys = Object.keys(item.data || {});
                if (keys.length === 0) return <span className="text-gray-500">-</span>;
                return <span title={keys.join('\n')}>{keys.length} key{keys.length > 1 ? 's' : ''}</span>;
            },
            getValue: (item: any) => Object.keys(item.data || {}).join(','),
        },
        {
            key: 'size',
            label: 'Size',
            defaultHidden: true,
            render: (item: any) => {
                // Secrets data is base64 encoded, so actual size is ~75% of stored size
                const total = Object.values(item.data || {}).reduce((sum: any, v: any) => sum + (v?.length || 0), 0) as number;
                const decoded = Math.floor(total * 0.75);
                if (decoded < 1024) return `~${decoded} B`;
                return `~${(decoded / 1024).toFixed(1)} KB`;
            },
            getValue: (item: any) => Object.values(item.data || {}).reduce((sum: any, v: any) => sum + (v?.length || 0), 0),
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <SecretActionsMenu
                    secret={item}
                    isOpen={activeMenuId === `secret-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `secret-${item.metadata.uid}`, buttonElement)}
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

    const helmToggle = (
        <label
            className="flex items-center gap-2 text-sm text-gray-400 hover:text-gray-300 cursor-pointer select-none no-drag"
            title="Helm releases can be managed in the Helm Releases section"
        >
            <input
                type="checkbox"
                checked={hideHelmSecrets}
                onChange={(e: any) => setHideHelmSecrets(e.target.checked)}
            />
            <span className="whitespace-nowrap">Hide Helm</span>
        </label>
    );

    return (
        <>
            <ResourceList
                title="Secrets"
                columns={columns}
                data={filteredSecrets}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="secrets"
                onRowClick={handleEditKeyValue}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
                customHeaderActions={helmToggle}
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
