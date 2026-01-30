import React from 'react';
import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import ServiceDetails from '../../../components/shared/ServiceDetails';

export const useServiceActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
    } = useBaseResourceActions({
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
