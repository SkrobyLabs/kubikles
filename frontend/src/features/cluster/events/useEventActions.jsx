import React from 'react';
import { useUI } from '../../../context/UIContext';
import { DeleteEvent } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useEventActions = () => {
    const { openTab, closeTab } = useUI();

    const handleEditYaml = (event) => {
        Logger.info("Opening event YAML editor", { namespace: event.metadata.namespace, name: event.metadata.name });
        const tabId = `yaml-event-${event.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${event.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={event.metadata.namespace}
                    resourceName={event.metadata.name}
                    isEvent={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = async (event) => {
        const name = event.metadata.name;
        const namespace = event.metadata.namespace;

        if (!confirm(`Are you sure you want to delete event "${name}"?`)) return;

        Logger.info("Deleting event", { namespace, name });
        try {
            await DeleteEvent(namespace, name);
            Logger.info("Delete triggered successfully", { namespace, name });
        } catch (err) {
            Logger.error("Failed to delete event", err);
            alert(`Failed to delete event: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
