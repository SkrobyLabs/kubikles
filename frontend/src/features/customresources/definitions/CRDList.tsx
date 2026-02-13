import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useCRDs } from '~/hooks/useCRDs';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteCRD, GetCRDYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import CRDActionsMenu from './CRDActionsMenu';
import { useCRDActions } from './useCRDActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function CRDList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { crds, loading } = useCRDs(currentContext, isVisible) as any;
    const { handleEditYaml } = useCRDActions();
    const selection = useSelection();

    // Wrap APIs to match useBulkActions signature (context, name) for cluster-scoped
    const deleteApi = useCallback((_context: any, name: any) => DeleteCRD(name), []);
    const getYamlApi = useCallback((name: any) => GetCRDYaml(name), []);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'CustomResourceDefinition',
        resourceType: 'crds',
        isNamespaced: false,
        deleteApi,
        getYamlApi,

    });

    // Get the served versions as a comma-separated string
    const getVersions = (crd: any) => {
        const versions = crd.spec?.versions || [];
        const served = versions.filter((v: any) => v.served).map((v: any) => v.name);
        return served.join(', ') || '-';
    };

    // Get the storage version (the one that is stored)
    const getStorageVersion = (crd: any) => {
        const versions = crd.spec?.versions || [];
        const storage = versions.find((v: any) => v.storage);
        return storage?.name || '-';
    };

    const columns = useMemo(() => [
        {
            key: 'resource',
            label: 'Resource',
            render: (item: any) => item.spec?.names?.kind || '-',
            getValue: (item: any) => item.spec?.names?.kind || ''
        },
        {
            key: 'group',
            label: 'Group',
            render: (item: any) => item.spec?.group || '-',
            getValue: (item: any) => item.spec?.group || ''
        },
        {
            key: 'version',
            label: 'Version',
            render: (item: any) => {
                const versions = item.spec?.versions || [];
                const storageVersion = versions.find((v: any) => v.storage);
                const servedVersions = versions.filter((v: any) => v.served && !v.storage && !v.deprecated);
                const deprecatedVersions = versions.filter((v: any) => v.served && v.deprecated);

                // Order: storage first, then served, then deprecated
                const orderedVersions = [
                    ...(storageVersion ? [storageVersion] : []),
                    ...servedVersions,
                    ...deprecatedVersions
                ];

                const maxDisplay = 3;
                const displayVersions = orderedVersions.slice(0, maxDisplay);
                const remaining = orderedVersions.length - maxDisplay;

                return (
                    <div className="flex flex-wrap gap-1 max-w-[200px]">
                        {displayVersions.map((v: any) => (
                            <span
                                key={v.name}
                                className={`text-xs px-1.5 py-0.5 rounded ${
                                    v.storage
                                        ? 'bg-blue-500/20 text-blue-400'
                                        : v.deprecated
                                            ? 'bg-yellow-500/20 text-yellow-400'
                                            : 'bg-gray-500/20 text-gray-400'
                                }`}
                                title={v.storage ? 'Storage version' : v.deprecated ? 'Deprecated' : 'Served'}
                            >
                                {v.name}
                            </span>
                        ))}
                        {remaining > 0 && (
                            <span className="text-xs text-gray-500">+{remaining}</span>
                        )}
                    </div>
                );
            },
            getValue: (item: any) => getStorageVersion(item)
        },
        {
            key: 'scope',
            label: 'Scope',
            render: (item: any) => item.spec?.scope || '-',
            getValue: (item: any) => item.spec?.scope || ''
        },
        {
            key: 'age',
            label: 'Age',
            render: (item: any) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item: any) => item.metadata?.creationTimestamp
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <CRDActionsMenu
                    crd={item}
                    isOpen={activeMenuId === `crd-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `crd-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(crd: any) => openBulkDelete([crd])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList
                title="Custom Resource Definitions"
                columns={columns}
                data={crds}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="crds"
                onRowClick={handleEditYaml}
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
