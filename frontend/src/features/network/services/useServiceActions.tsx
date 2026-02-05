import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import ServiceDetails from '../../../components/shared/ServiceDetails';
import { K8sService } from '../../../types/k8s';

export const useServiceActions = (): Pick<
    BaseResourceActionsReturn<K8sService>,
    'handleEditYaml' | 'handleShowDependencies' | 'handleShowDetails'
> => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
    } = useBaseResourceActions<K8sService>({
        resourceType: 'service',
        resourceLabel: 'Service',
        DetailsComponent: ServiceDetails,
        detailsPropName: 'service',
    });

    return {
        handleEditYaml,
        handleShowDependencies,
        handleShowDetails,
    };
};
