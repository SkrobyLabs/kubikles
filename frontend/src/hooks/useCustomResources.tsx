import { useMemo } from 'react';
import { ListCustomResources } from 'wailsjs/go/main/App';
import { useCRDWatcher } from './useResourceWatcher';
import { createNamespacedResourceHook } from './useResource';
import { K8sResource } from '../types/k8s';

interface UseCustomResourcesResult {
    resources: K8sResource[];
    loading: boolean;
    error: Error | null;
}

/** Custom resources use the same query planning, cancellation and filtering as built-in resources. */
export const useCustomResources = (
    currentContext: string | null,
    group: string,
    version: string,
    resource: string,
    selectedNamespaces: string[],
    isVisible: boolean,
    isNamespaced: boolean
): UseCustomResourcesResult => {
    const useResources = useMemo(() => createNamespacedResourceHook<K8sResource>(
        `crd:${group}/${version}/${resource}`,
        (id, namespace) => ListCustomResources(id, group, version, resource, namespace),
        'resources',
        (_type, namespaces, onEvent, enabled, onError) => useCRDWatcher(group, version, resource, namespaces, onEvent, enabled, onError),
    ), [group, version, resource]);
    const result = useResources(currentContext, isNamespaced ? selectedNamespaces : ['*'],
        Boolean(isVisible && group && version && resource));
    return { resources: result.resources as K8sResource[], loading: result.loading, error: result.error };
};
