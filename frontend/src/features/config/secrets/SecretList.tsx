import React, { useMemo, useState } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
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
import { HELM_RELEASE_SECRET_TYPE, secretSemantics } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildSecretViewColumns } from './viewColumns';

const SECRET_INITIAL_SORT = { key: 'age', direction: 'desc' };
// The view path has no Size column: secret data never leaves the API server, only its metadata does.
const SECRET_VIEW_COLUMNS = defaultViewColumns('secrets', ['keys']);
const NO_DATA: any[] = [];

export default function SecretList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const [hideHelmSecrets, setHideHelmSecrets] = useState(true);
    const viewOptions = useMemo(() => ({
        sort: SECRET_INITIAL_SORT,
        hide: hideHelmSecrets ? ['helm'] : [],
        columns: SECRET_VIEW_COLUMNS,
    }), [hideHelmSecrets]);
    const { secrets, loading, loadState, view } = useSecrets(currentContext, selectedNamespaces, isVisible, viewOptions) as any;

    // Filter out Helm release secrets if toggle is enabled
    const filteredSecrets = useMemo(() => {
        if (!hideHelmSecrets) return secrets;
        return secrets.filter((secret: any) => secret.type !== HELM_RELEASE_SECRET_TYPE);
    }, [secrets, hideHelmSecrets]);
    const { handleEditYaml, handleEditKeyValue, handleShowDependencies } = useStableCallbacks(useSecretActions());
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
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: secretSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: secretSemantics.namespace.getValue },
        { key: 'type', label: 'Type', render: (item: any) => item.type, getValue: secretSemantics.type.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: secretSemantics.age.getValue },
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
            getValue: secretSemantics.keys.getValue,
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
            getValue: secretSemantics.size.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={SecretActionsMenu}
                    menuId={`secret-${item.metadata.uid}`}
                    secret={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(item: any) => openBulkDelete([item])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildSecretViewColumns({ onEditYaml: handleEditYaml, onShowDependencies: handleShowDependencies, onDelete: (item: any) => openBulkDelete([item]) }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

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
            <ResourceList loadState={loadState}
                title="Secrets"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : filteredSecrets}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={SECRET_INITIAL_SORT}
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
