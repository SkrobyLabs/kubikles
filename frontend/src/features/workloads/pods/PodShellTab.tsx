import React, { useState } from 'react';
import { LazyTerminal as Terminal } from '~/components/lazy';
import ContainerSelector from '~/components/shared/ContainerSelector';

/**
 * Get container name from container (supports both string and object format)
 */
const getContainerName = (container) => {
    return typeof container === 'object' ? container.name : container;
};

/**
 * Pod shell tab with inline container selection.
 * Shows container selector when multiple containers are available.
 */
const PodShellTab = ({ namespace, pod, containers = [], context }) => {
    // If only one container, go straight to terminal
    const [state, setState] = useState(containers.length > 1 ? 'selecting' : 'connected');
    const [selectedContainer, setSelectedContainer] = useState(
        containers.length === 1 ? getContainerName(containers[0]) : ''
    );

    const handleContainerSelect = (containerName) => {
        setSelectedContainer(containerName);
        setState('connected');
    };

    if (state === 'selecting') {
        return (
            <ContainerSelector
                containers={containers}
                podName={pod}
                title="Select Container"
                description={<>Choose a container to open shell in <span className="font-medium text-foreground">{pod}</span></>}
                onSelect={handleContainerSelect}
            />
        );
    }

    return (
        <Terminal
            namespace={namespace}
            pod={pod}
            container={selectedContainer}
            context={context}
        />
    );
};

export default PodShellTab;
