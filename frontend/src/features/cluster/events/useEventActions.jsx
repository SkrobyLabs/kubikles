import React from 'react';
import { useUI } from '../../../context/UIContext';
import { DeleteEvent } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useEventActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();

    const handleEditYaml = (event) => {
        Logger.info("Opening event YAML editor", { namespace: event.metadata.namespace, name: event.metadata.name });
        const tabId = `yaml-event-${event.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${event.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="event"
                    namespace={event.metadata.namespace}
                    resourceName={event.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = (event) => {
        const name = event.metadata.name;
        const namespace = event.metadata.namespace;
        Logger.info("Delete Event requested", { namespace, name });

        openModal({
            title: `Delete Event ${name}?`,
            content: `Are you sure you want to delete event "${name}"?`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteEvent(namespace, name);
                    Logger.info("Event deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete event", err);
                    alert(`Failed to delete event: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
