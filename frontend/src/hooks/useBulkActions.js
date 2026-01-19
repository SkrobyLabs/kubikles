import { useState, useCallback } from 'react';
import { SaveYamlBackup } from '../../wailsjs/go/main/App';
import Logger from '../utils/Logger';

/**
 * Hook for managing bulk actions (delete, restart) on resources.
 * Provides unified state management and handlers for BulkActionModal.
 *
 * Also supports single-resource deletion by passing a single-item array,
 * providing consistent UX for both single and bulk operations.
 *
 * @param {Object} config - Configuration object
 * @param {string} config.resourceLabel - Human-readable label (e.g., 'ConfigMap', 'Deployment')
 * @param {string} config.resourceType - Resource type for backup filename (e.g., 'configmaps', 'deployments')
 * @param {boolean} [config.isNamespaced=true] - Whether the resource is namespaced
 * @param {Function} config.deleteApi - Delete function: (context, namespace, name) or (context, name) for cluster-scoped
 * @param {Function} [config.restartApi] - Optional restart function with same signature as deleteApi
 * @param {Function} config.getYamlApi - Get YAML function: (namespace, name) or (name) for cluster-scoped
 * @param {string} config.currentContext - Current K8s context
 * @returns {Object} Bulk action state and handlers
 */
export function useBulkActions(config) {
    const {
        resourceLabel,
        resourceType,
        isNamespaced = true,
        deleteApi,
        restartApi,
        getYamlApi,
        currentContext,
    } = config;

    // Modal state
    const [bulkActionModal, setBulkActionModal] = useState({
        isOpen: false,
        action: null, // 'delete' | 'restart'
        items: [],
    });

    // Progress state
    const [bulkProgress, setBulkProgress] = useState({
        current: 0,
        total: 0,
        status: 'idle', // 'idle' | 'inProgress' | 'complete'
        results: [],
    });

    /**
     * Open bulk action modal for delete
     * @param {Array} items - Array of resources to delete (can be single item)
     */
    const openBulkDelete = useCallback((items) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items });
        setBulkProgress({ current: 0, total: items.length, status: 'idle', results: [] });
    }, []);

    /**
     * Open bulk action modal for restart (if supported)
     * @param {Array} items - Array of resources to restart
     */
    const openBulkRestart = useCallback((items) => {
        if (!restartApi) {
            Logger.warn('Restart not supported for this resource type');
            return;
        }
        setBulkActionModal({ isOpen: true, action: 'restart', items });
        setBulkProgress({ current: 0, total: items.length, status: 'idle', results: [] });
    }, [restartApi]);

    /**
     * Close bulk action modal and reset state
     */
    const closeBulkAction = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    /**
     * Execute bulk action on items
     * @param {Array} items - Array of resources to act on
     */
    const confirmBulkAction = useCallback(async (items) => {
        const action = bulkActionModal.action;
        const api = action === 'delete' ? deleteApi : restartApi;

        if (!api) {
            Logger.error(`No API configured for action: ${action}`);
            return;
        }

        Logger.info(`Bulk ${action} started`, { resourceType, count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));

        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                if (isNamespaced) {
                    await api(currentContext, namespace, name);
                } else {
                    await api(currentContext, name);
                }
                results.push({ name, namespace: namespace || '', success: true, message: '' });
                Logger.info(`${resourceLabel} ${action}d`, { namespace, name });
            } catch (err) {
                results.push({ name, namespace: namespace || '', success: false, message: err.toString() });
                Logger.error(`Failed to ${action} ${resourceLabel.toLowerCase()}`, { namespace, name, error: err });
            }

            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }

        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
        Logger.info(`Bulk ${action} completed`, {
            resourceType,
            total: items.length,
            success: results.filter(r => r.success).length,
            failed: results.filter(r => !r.success).length,
        });
    }, [bulkActionModal.action, deleteApi, restartApi, isNamespaced, currentContext, resourceLabel, resourceType]);

    /**
     * Export YAML backup for items
     * @param {Array} items - Array of resources to export
     */
    const exportYaml = useCallback(async (items) => {
        Logger.info('Exporting YAML backup', { resourceType, count: items.length });

        const entries = [];
        for (const item of items) {
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                const yaml = isNamespaced
                    ? await getYamlApi(namespace, name)
                    : await getYamlApi(name);
                entries.push({ namespace: namespace || '', name, kind: resourceLabel, yaml });
                Logger.info('Fetched YAML for backup', { namespace, name });
            } catch (err) {
                Logger.error('Failed to get YAML for backup', { namespace, name, error: err });
                entries.push({ namespace: namespace || '', name, kind: resourceLabel, yaml: `# Failed to fetch YAML: ${err}` });
            }
        }

        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        const filename = `${resourceType}-backup-${timestamp}.zip`;

        try {
            await SaveYamlBackup(entries, filename);
            Logger.info('YAML backup saved', { filename });
        } catch (err) {
            Logger.error('Failed to save YAML backup', { error: err });
            // Don't use alert - errors are shown in the modal results
            if (err && err.toString() !== '') {
                Logger.error('Backup save failed', { error: err.toString() });
            }
        }
    }, [resourceType, resourceLabel, isNamespaced, getYamlApi]);

    return {
        // State
        bulkActionModal,
        bulkProgress,
        // Handlers
        openBulkDelete,
        openBulkRestart: restartApi ? openBulkRestart : undefined,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
        // Convenience: check if restart is supported
        supportsRestart: !!restartApi,
    };
}
