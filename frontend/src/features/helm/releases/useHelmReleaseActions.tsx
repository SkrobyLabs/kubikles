import React, { useCallback } from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { UninstallHelmRelease } from 'wailsjs/go/main/App';
import HelmReleaseDetails from './HelmReleaseDetails';
import Logger from '~/utils/Logger';
import { K8sHelmRelease } from '~/types/k8s';

/**
 * Return type for useHelmReleaseActions
 */
export interface HelmReleaseActionsReturn {
    handleOpenDetails: (release: K8sHelmRelease, initialTab?: string) => void;
    handleViewValues: (release: K8sHelmRelease) => void;
    handleViewHistory: (release: K8sHelmRelease) => void;
    handleRollback: (release: K8sHelmRelease) => void;
    handleUninstall: (release: K8sHelmRelease) => void;
}

export const useHelmReleaseActions = (): any => {
    const { openTab, openModal, closeModal } = useUI();
    const { currentContext, triggerRefresh } = useK8s();
    const { addNotification } = useNotification();

    // Open details tab with specific initial tab
    const handleOpenDetails = useCallback((release: K8sHelmRelease, initialTab: string = 'basic'): void => {
        Logger.info("Open details for Helm release", { namespace: release.namespace, name: release.name, initialTab }, 'helm');
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

    const handleViewValues = useCallback((release: K8sHelmRelease): void => {
        Logger.info("View values for Helm release", { namespace: release.namespace, name: release.name }, 'helm');
        // Open details tab with values tab active
        handleOpenDetails(release, 'values');
    }, [handleOpenDetails]);

    const handleViewHistory = useCallback((release: K8sHelmRelease): void => {
        Logger.info("View history for Helm release", { namespace: release.namespace, name: release.name }, 'helm');
        // Open details tab with history tab active
        handleOpenDetails(release, 'history');
    }, [handleOpenDetails]);

    // Rollback now just opens the history tab where user can select a revision
    const handleRollback = useCallback((release: K8sHelmRelease): void => {
        Logger.info("Rollback requested for Helm release", { namespace: release.namespace, name: release.name }, 'helm');
        // Open details tab with history tab active - user can pick revision there
        handleOpenDetails(release, 'history');
    }, [handleOpenDetails]);

    const handleUninstall = useCallback((release: K8sHelmRelease): void => {
        const name = release.name;
        const namespace = release.namespace;
        Logger.info("Uninstall Helm release requested", { namespace, name }, 'helm');

        openModal({
            title: `Uninstall ${name}?`,
            content: `Are you sure you want to uninstall Helm release "${name}" in namespace "${namespace}"? This action cannot be undone.`,
            confirmText: 'Uninstall',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await UninstallHelmRelease(namespace, name);
                    Logger.info("Helm release uninstalled successfully", { namespace, name }, 'helm');
                    closeModal();
                    triggerRefresh();
                } catch (err: any) {
                    Logger.error("Failed to uninstall Helm release", err, 'helm');
                    addNotification({ type: 'error', title: 'Failed to uninstall', message: String(err.message || err) });
                }
            }
        });
    }, [openModal, closeModal, triggerRefresh, addNotification]);

    return {
        handleOpenDetails,
        handleViewValues,
        handleViewHistory,
        handleRollback,
        handleUninstall
    };
};
