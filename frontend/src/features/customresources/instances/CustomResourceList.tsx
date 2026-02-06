import React, { useMemo, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useCustomResources } from '~/hooks/useCustomResources';
import { useCRDPrinterColumns } from '~/hooks/useCRDPrinterColumns';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteCustomResource, GetCustomResourceYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon, CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import CustomResourceActionsMenu from './CustomResourceActionsMenu';
import { useCustomResourceActions } from './useCustomResourceActions';
import { useMenuPosition } from '~/hooks/useMenuPosition';

/**
 * Evaluates a JSONPath expression against an object.
 * Supports common patterns:
 * - .field.subfield
 * - .field[index]
 * - .field[?(@.key=="value")].subfield (simple filter)
 */
const evaluateJSONPath = (obj, jsonPath) => {
    if (!obj || !jsonPath) return undefined;

    // Remove leading dot if present
    let path = jsonPath.startsWith('.') ? jsonPath.substring(1) : jsonPath;

    // Handle filter expressions like [?(@.type=="Ready")]
    const filterMatch = path.match(/^([^[]+)\[\?\(@\.([^=]+)==["']([^"']+)["']\)\]\.?(.*)$/);
    if (filterMatch) {
        const [, arrayPath, filterKey, filterValue, remainingPath] = filterMatch;
        const arr = evaluateJSONPath(obj, arrayPath);
        if (!Array.isArray(arr)) return undefined;
        const item = arr.find(i => i && i[filterKey] === filterValue);
        if (!item) return undefined;
        return remainingPath ? evaluateJSONPath(item, remainingPath) : item;
    }

    // Handle simple paths
    const parts = path.split(/\.|\[|\]/).filter(p => p !== '');
    let current = obj;

    for (const part of parts) {
        if (current === null || current === undefined) return undefined;
        // Handle array index
        if (/^\d+$/.test(part)) {
            current = current[parseInt(part, 10)];
        } else {
            current = current[part];
        }
    }

    return current;
};

/**
 * Renders a value based on its type from the CRD column definition
 */
const renderColumnValue = (value, type) => {
    if (value === undefined || value === null) return '-';

    switch (type) {
        case 'date':
            return formatAge(value);
        case 'boolean':
            // Show a colored indicator for boolean values
            if (value === true || value === 'True' || value === 'true') {
                return <CheckCircleIcon className="h-5 w-5 text-green-400" />;
            } else if (value === false || value === 'False' || value === 'false') {
                return <XCircleIcon className="h-5 w-5 text-red-400" />;
            }
            return String(value);
        case 'integer':
        case 'number':
            return String(value);
        default:
            // For status-like values, add color coding
            if (typeof value === 'string') {
                if (value === 'True' || value === 'Ready' || value === 'Active' || value === 'Bound' || value === 'Running') {
                    return <span className="text-green-400">{value}</span>;
                } else if (value === 'False' || value === 'Failed' || value === 'Error') {
                    return <span className="text-red-400">{value}</span>;
                } else if (value === 'Pending' || value === 'Unknown' || value === 'Progressing') {
                    return <span className="text-yellow-400">{value}</span>;
                }
            }
            return String(value);
    }
};

/**
 * Generic list component for custom resource instances
 * @param {Object} props
 * @param {Object} props.crdInfo - CRD information: { group, version, resource, kind, namespaced, plural }
 * @param {boolean} props.isVisible - Whether this component is visible (for data fetching)
 */
export default function CustomResourceList({ crdInfo, isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { handleEditYaml } = useCustomResourceActions(crdInfo);
    const selection = useSelection();

    // Wrap APIs to match useBulkActions signature
    // For namespaced: (context, namespace, name), for cluster-scoped: (context, name)
    const deleteApi = useCallback((_context, namespaceOrName, maybeName) => {
        if (crdInfo.namespaced) {
            return DeleteCustomResource(crdInfo.group, crdInfo.version, crdInfo.resource, namespaceOrName, maybeName);
        } else {
            // For cluster-scoped, namespaceOrName is actually the name
            return DeleteCustomResource(crdInfo.group, crdInfo.version, crdInfo.resource, '', namespaceOrName);
        }
    }, [crdInfo.group, crdInfo.version, crdInfo.resource, crdInfo.namespaced]);

    const getYamlApi = useCallback((namespaceOrName, maybeName) => {
        if (crdInfo.namespaced) {
            return GetCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, namespaceOrName, maybeName);
        } else {
            // For cluster-scoped, namespaceOrName is actually the name
            return GetCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, '', namespaceOrName);
        }
    }, [crdInfo.group, crdInfo.version, crdInfo.resource, crdInfo.namespaced]);

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: crdInfo.kind,
        resourceType: crdInfo.resource,
        isNamespaced: crdInfo.namespaced,
        deleteApi,
        getYamlApi,

    });

    // Fetch CRD printer columns
    const { columns: printerColumns } = useCRDPrinterColumns(crdInfo.group, crdInfo.resource, isVisible);

    const { resources, loading, error } = useCustomResources(
        currentContext,
        crdInfo.group,
        crdInfo.version,
        crdInfo.resource,
        selectedNamespaces,
        isVisible,
        crdInfo.namespaced
    );

    // Show namespace column when viewing all namespaces or multiple namespaces (not exactly 1)
    const showNamespaceColumn = crdInfo.namespaced && (selectedNamespaces?.length !== 1);

    const columns = useMemo(() => {
        const cols = [
            {
                key: 'name',
                label: 'Name',
                render: (item) => item.metadata?.name || '-',
                getValue: (item) => item.metadata?.name || ''
            }
        ];

        // Add namespace column when viewing multiple/all namespaces
        if (showNamespaceColumn) {
            cols.push({
                key: 'namespace',
                label: 'Namespace',
                render: (item) => item.metadata?.namespace || '-',
                getValue: (item) => item.metadata?.namespace || ''
            });
        }

        // Add dynamic columns from CRD additionalPrinterColumns
        // Skip columns that we already handle (Name, Age) or that point to metadata we show
        const skipColumns = new Set(['name', 'age', 'namespace']);

        for (const col of printerColumns) {
            const colKey = col.name.toLowerCase().replace(/\s+/g, '_');

            // Skip if it's a standard column we already handle
            if (skipColumns.has(colKey)) continue;

            // Skip Age column if it's pointing to creationTimestamp (we handle it specially)
            if (col.jsonPath === '.metadata.creationTimestamp') continue;

            cols.push({
                key: colKey,
                label: col.name,
                render: (item) => {
                    const value = evaluateJSONPath(item, col.jsonPath);
                    return renderColumnValue(value, col.type);
                },
                getValue: (item) => {
                    const value = evaluateJSONPath(item, col.jsonPath);
                    if (value === undefined || value === null) return '';
                    return String(value);
                }
            });
        }

        // Add age column
        cols.push({
            key: 'age',
            label: 'Age',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp
        });

        // Add actions column
        cols.push({
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <CustomResourceActionsMenu
                    resource={item}
                    isOpen={activeMenuId === `cr-${item.metadata?.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `cr-${item.metadata?.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(resource) => openBulkDelete([resource])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        });

        return cols;
    }, [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete, showNamespaceColumn, printerColumns]);

    return (
        <>
            <ResourceList
                title={crdInfo.kind}
                columns={columns}
                data={resources}
                isLoading={loading}
                namespaces={crdInfo.namespaced ? namespaces : []}
                currentNamespace={crdInfo.namespaced ? selectedNamespaces : []}
                onNamespaceChange={crdInfo.namespaced ? setSelectedNamespaces : undefined}
                showNamespaceSelector={crdInfo.namespaced}
                multiSelectNamespaces={true}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType={`cr-${crdInfo.group}-${crdInfo.resource}`}
                onRowClick={handleEditYaml}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
