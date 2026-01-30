import React, { useCallback } from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { useNotification } from '../../../context/NotificationContext';
import { UninstallHelmRelease } from '../../../../wailsjs/go/main/App';
import HelmReleaseDetails from './HelmReleaseDetails';
import Logger from '../../../utils/Logger';

export const useHelmReleaseActions = () => {
    const { openTab, openModal, closeModal } = useUI();
    const { currentContext, triggerRefresh } = useK8s();
    const { addNotification } = useNotification();

    // Open details tab with specific initial tab
    const handleOpenDetails = useCallback((release, initialTab = 'basic') => {
        Logger.info("Open details for Helm release", { namespace: release.namespace, name: release.name, initialTab });
        const tabId = `helm-details-${release.namespace}-${release.name}`;
        openTab({
            id: tabId,
            title: `${release.name}`,
            content: (
                <HelmReleaseDetails
                    release={release}
                    tabContext={currentContext}
                    initialTab={initialTab}
                />
            ),
            resourceMeta: { kind: 'HelmRelease', name: release.name, namespace: release.namespace },
        });
    }, [openTab, currentContext]);

    const handleViewValues = useCallback((release) => {
        Logger.info("View values for Helm release", { namespace: release.namespace, name: release.name });
        // Open details tab with values tab active
        handleOpenDetails(release, 'values');
    }, [handleOpenDetails]);

    const handleViewHistory = useCallback((release) => {
        Logger.info("View history for Helm release", { namespace: release.namespace, name: release.name });
        // Open details tab with history tab active
        handleOpenDetails(release, 'history');
    }, [handleOpenDetails]);

    // Rollback now just opens the history tab where user can select a revision
    const handleRollback = useCallback((release) => {
        Logger.info("Rollback requested for Helm release", { namespace: release.namespace, name: release.name });
        // Open details tab with history tab active - user can pick revision there
        handleOpenDetails(release, 'history');
    }, [handleOpenDetails]);

    const handleUninstall = useCallback((release) => {
        const name = release.name;
        const namespace = release.namespace;
        Logger.info("Uninstall Helm release requested", { namespace, name });

        openModal({
            title: `Uninstall ${name}?`,
            content: `Are you sure you want to uninstall Helm release "${name}" in namespace "${namespace}"? This action cannot be undone.`,
            confirmText: 'Uninstall',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await UninstallHelmRelease(namespace, name);
                    Logger.info("Helm release uninstalled successfully", { namespace, name });
                    closeModal();
                    triggerRefresh();
                } catch (err) {
                    Logger.error("Failed to uninstall Helm release", err);
                    addNotification({ type: 'error', title: 'Failed to uninstall', message: String(err.message || err) });
                }
            }
        });
    }, [openModal, closeModal, triggerRefresh]);

    return {
        handleOpenDetails,
        handleViewValues,
        handleViewHistory,
        handleRollback,
        handleUninstall
    };
};
