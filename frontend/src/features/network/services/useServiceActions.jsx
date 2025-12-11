import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import ServiceDetails from '../../../components/shared/ServiceDetails';
import Logger from '../../../utils/Logger';

export const useServiceActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (service) => {
        Logger.info("Opening YAML editor for Service", { namespace: service.metadata.namespace, name: service.metadata.name });
        const tabId = `yaml-service-${service.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${service.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="service"
                    namespace={service.metadata.namespace}
                    resourceName={service.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (service) => {
        Logger.info("Opening dependency graph", { namespace: service.metadata.namespace, service: service.metadata.name });
        const tabId = `deps-service-${service.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${service.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="service"
                    namespace={service.metadata.namespace}
                    resourceName={service.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleShowDetails = (service) => {
        Logger.info("Opening service details", { namespace: service.metadata.namespace, name: service.metadata.name });
        const tabId = `details-service-${service.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${service.metadata.name}`,
            content: (
                <ServiceDetails
                    service={service}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    return {
        handleEditYaml,
        handleShowDependencies,
        handleShowDetails
    };
};
