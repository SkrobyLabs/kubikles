import { useOperatorActions } from '../actions/useOperatorActions';
import React, { useMemo, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useCustomResources } from '~/hooks/useCustomResources';
import { useCRDPrinterColumns } from '~/hooks/useCRDPrinterColumns';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteCustomResource, GetCustomResourceYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import CustomResourceActionsMenu from './CustomResourceActionsMenu';
import { useCustomResourceActions } from './useCustomResourceActions';
import { renderColumnValue } from './renderColumnValue';
import { buildCustomResourceViewColumns } from './viewColumns';
import { useRemoteOrLegacy } from '~/hooks/useRemoteOrLegacy';
import { useWithFullObject } from '~/hooks/useViewObject';

const CRD_INITIAL_SORT = { key: 'age', direction: 'desc' };
const NO_DATA: any[] = [];

/**
 * Evaluates a JSONPath expression against an object.
 * Supports common patterns:
 * - .field.subfield
 * - .field[index]
 * - .field[?(@.key=="value")].subfield (simple filter)
 */
const evaluateJSONPath = (obj: any, jsonPath: any): any => {
    if (!obj || !jsonPath) return undefined;

    // Remove leading dot if present
    let path = jsonPath.startsWith('.') ? jsonPath.substring(1) : jsonPath;

    // Handle filter expressions like [?(@.type=="Ready")]
    const filterMatch = path.match(/^([^[]+)\[\?\(@\.([^=]+)==["']([^"']+)["']\)\]\.?(.*)$/);
    if (filterMatch) {
        const [, arrayPath, filterKey, filterValue, remainingPath] = filterMatch;
        const arr = evaluateJSONPath(obj, arrayPath);
        if (!Array.isArray(arr)) return undefined;
        const item = arr.find((i: any) => i && i[filterKey] === filterValue);
        if (!item) return undefined;
        return remainingPath ? evaluateJSONPath(item, remainingPath) : item;
    }

    // Handle simple paths
    const parts = path.split(/\.|\[|\]/).filter((p: any) => p !== '');
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
 * Generic list component for custom resource instances
 * @param {Object} props
 * @param {Object} props.crdInfo - CRD information: { group, version, resource, kind, namespaced, plural }
 * @param {boolean} props.isVisible - Whether this component is visible (for data fetching)
 */
export default function CustomResourceList({ crdInfo, isVisible }: any) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useCustomResourceActions(crdInfo));
    const selection = useSelection();
    const operator = useOperatorActions(crdInfo, currentContext);

    // Wrap APIs to match useBulkActions signature:
    // namespaced: (namespace, name), cluster-scoped: (name)
    const deleteApi = useCallback((namespaceOrName: any, maybeName?: any) => {
        if (crdInfo.namespaced) {
            return DeleteCustomResource(crdInfo.group, crdInfo.version, crdInfo.resource, namespaceOrName, maybeName);
        } else {
            // For cluster-scoped, namespaceOrName is actually the name
            return DeleteCustomResource(crdInfo.group, crdInfo.version, crdInfo.resource, '', namespaceOrName);
        }
    }, [crdInfo.group, crdInfo.version, crdInfo.resource, crdInfo.namespaced]);

    const getYamlApi = useCallback((namespaceOrName: any, maybeName: any) => {
        if (crdInfo.namespaced) {
            return GetCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, namespaceOrName, maybeName);
        } else {
            // For cluster-scoped, namespaceOrName is actually the name
            return GetCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, '', namespaceOrName);
        }
    }, [crdInfo.group, crdInfo.version, crdInfo.resource, crdInfo.namespaced]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: crdInfo.kind,
        resourceType: crdInfo.resource,
        isNamespaced: crdInfo.namespaced,
        deleteApi: deleteApi as any,
        getYamlApi,

    });

    // Fetch CRD printer columns
    const { columns: printerColumns } = useCRDPrinterColumns(crdInfo.group, crdInfo.resource, isVisible);

    // Windowed list when the viewStore setting is on: the backend builds the row schema from the
    // CRD's printer columns. Otherwise (or when it fails) the legacy hook feeds the list.
    const crdType = `crd:${crdInfo.group}/${crdInfo.version}/${crdInfo.resource}`;
    const crdDescriptor = useMemo(() => ({
        group: crdInfo.group, version: crdInfo.version, resource: crdInfo.resource, namespaced: !!crdInfo.namespaced,
    }), [crdInfo.group, crdInfo.version, crdInfo.resource, crdInfo.namespaced]);
    const viewOptions = useMemo(() => ({ sort: CRD_INITIAL_SORT, crd: crdDescriptor }), [crdDescriptor]);
    const { resources, loading, loadState, view, viewSnapshot } = useRemoteOrLegacy(
        crdType,
        'resources',
        (visible: boolean) => useCustomResources(
            currentContext,
            crdInfo.group,
            crdInfo.version,
            crdInfo.resource,
            selectedNamespaces,
            visible,
            crdInfo.namespaced
        ),
        {
            context: currentContext,
            namespaces: crdInfo.namespaced ? selectedNamespaces : ['*'],
            isVisible: !!isVisible,
            ...viewOptions,
        },
    ) as any;
    // Show namespace column when viewing all namespaces or multiple namespaces (not exactly 1)
    const showNamespaceColumn = crdInfo.namespaced && (selectedNamespaces?.length !== 1 || selectedNamespaces[0] === '*');
    const withObject = useWithFullObject(view ?? null);
    // Details and YAML editing read the whole resource; rows of a windowed list fetch it at click time.
    const showDetailsFull = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);
    const editYamlFull = useMemo(() => withObject(handleEditYaml), [withObject, handleEditYaml]);
    const viewColumns = useMemo(
        () => buildCustomResourceViewColumns(viewSnapshot?.schema, {
            onShowDetails: showDetailsFull, onEditYaml: editYamlFull, onDelete: (resource: any) => openBulkDelete([resource]),
            operatorActions: operator.actions, onOperatorAction: operator.openAction,
        }).filter((c: any) => c.key !== 'namespace' || showNamespaceColumn),
        [viewSnapshot?.schema, showDetailsFull, editYamlFull, openBulkDelete, operator.actions, operator.openAction, showNamespaceColumn]);


    const columns = useMemo(() => {
        const cols: any[] = [
            {
                key: 'name',
                label: 'Name',
                render: (item: any) => item.metadata?.name || '-',
                getValue: (item: any) => item.metadata?.name || ''
            }
        ];

        // Add namespace column when viewing multiple/all namespaces
        if (showNamespaceColumn) {
            cols.push({
                key: 'namespace',
                label: 'Namespace',
                render: (item: any) => item.metadata?.namespace || '-',
                getValue: (item: any) => item.metadata?.namespace || ''
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
                render: (item: any) => {
                    const value = evaluateJSONPath(item, col.jsonPath);
                    return renderColumnValue(value, col.type);
                },
                getValue: (item: any) => {
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
            render: (item: any) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item: any) => item.metadata?.creationTimestamp
        });

        // Add actions column
        cols.push({
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={CustomResourceActionsMenu}
                    menuId={`cr-${item.metadata?.uid}`}
                    resource={item}
                    operatorActions={operator.actions}
                    onOperatorAction={operator.openAction}
                    onShowDetails={handleShowDetails}
                    onEditYaml={handleEditYaml}
                    onDelete={(resource: any) => openBulkDelete([resource])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        });

        return cols;
    }, [handleShowDetails, handleEditYaml, openBulkDelete, showNamespaceColumn, printerColumns, operator.actions, operator.openAction]);

    return (
        <>
            <ResourceList loadState={loadState}
                title={crdInfo.kind}
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : resources}
                isLoading={loading}
                namespaces={crdInfo.namespaced ? namespaces : []}
                currentNamespace={crdInfo.namespaced ? selectedNamespaces : []}
                onNamespaceChange={crdInfo.namespaced ? setSelectedNamespaces : undefined}
                showNamespaceSelector={crdInfo.namespaced}
                multiSelectNamespaces={true}
                initialSort={CRD_INITIAL_SORT}
                resourceType={`cr-${crdInfo.group}-${crdInfo.resource}`}
                selectionLabel={crdInfo.resource}
                onRowClick={view ? showDetailsFull : handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            {operator.dialog}
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </>
    );
}
