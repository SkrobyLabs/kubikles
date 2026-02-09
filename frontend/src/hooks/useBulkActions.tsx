import { useState, useCallback } from 'react';
import { SaveYamlBackup } from 'wailsjs/go/main/App';
import { main } from 'wailsjs/go/models';
import Logger from '../utils/Logger';

// Resource metadata interface matching K8s ObjectMeta
interface ResourceMetadata {
    namespace?: string;
    name: string;
    uid?: string;
}

// Generic resource interface with metadata
interface Resource {
    metadata?: ResourceMetadata;
}

// Bulk action type
type BulkActionType = 'delete' | 'restart';

// Modal state
interface BulkActionModalState {
    isOpen: boolean;
    action: BulkActionType | null;
    items: Resource[];
}

// Result of a single bulk action operation
interface BulkActionResult {
    name: string;
    namespace: string;
    success: boolean;
    message: string;
}

// Progress tracking state
interface BulkProgressState {
    current: number;
    total: number;
    status: 'idle' | 'inProgress' | 'complete';
    results: BulkActionResult[];
}

// API function signatures
type NamespacedApiFn = (namespace: string, name: string) => Promise<void>;
type ClusterScopedApiFn = (name: string) => Promise<void>;
type GetYamlApiFn = ((namespace: string, name: string) => Promise<string>) | ((name: string) => Promise<string>);

// Configuration for the hook
interface UseBulkActionsConfig {
    resourceLabel: string;
    resourceType: string;
    isNamespaced?: boolean;
    deleteApi: NamespacedApiFn | ClusterScopedApiFn;
    restartApi?: NamespacedApiFn | ClusterScopedApiFn;
    getYamlApi: GetYamlApiFn;
}

// Export options for YAML backup
interface ExportYamlOptions {
    onProgress?: (current: number, total: number) => void;
    signal?: AbortSignal;
}

// Return type of the hook
interface UseBulkActionsReturn {
    bulkActionModal: BulkActionModalState;
    bulkProgress: BulkProgressState;
    openBulkDelete: (items: Resource[]) => void;
    openBulkRestart?: (items: Resource[]) => void;
    closeBulkAction: () => void;
    confirmBulkAction: (items: Resource[]) => Promise<void>;
    exportYaml: (items: Resource[], options?: ExportYamlOptions) => Promise<void>;
    supportsRestart: boolean;
}

/**
 * Hook for managing bulk actions (delete, restart) on resources.
 * Provides unified state management and handlers for BulkActionModal.
 *
 * Also supports single-resource deletion by passing a single-item array,
 * providing consistent UX for both single and bulk operations.
 */
export function useBulkActions(config: UseBulkActionsConfig): UseBulkActionsReturn {
    const {
        resourceLabel,
        resourceType,
        isNamespaced = true,
        deleteApi,
        restartApi,
        getYamlApi,
    } = config;

    // Modal state
    const [bulkActionModal, setBulkActionModal] = useState<BulkActionModalState>({
        isOpen: false,
        action: null,
        items: [],
    });

    // Progress state
    const [bulkProgress, setBulkProgress] = useState<BulkProgressState>({
        current: 0,
        total: 0,
        status: 'idle',
        results: [],
    });

    /**
     * Open bulk action modal for delete
     */
    const openBulkDelete = useCallback((items: Resource[]): void => {
        setBulkActionModal({ isOpen: true, action: 'delete', items });
        setBulkProgress({ current: 0, total: items.length, status: 'idle', results: [] });
    }, []);

    /**
     * Open bulk action modal for restart (if supported)
     */
    const openBulkRestart = useCallback((items: Resource[]): void => {
        if (!restartApi) {
            Logger.warn('Restart not supported for this resource type', undefined, 'k8s');
            return;
        }
        setBulkActionModal({ isOpen: true, action: 'restart', items });
        setBulkProgress({ current: 0, total: items.length, status: 'idle', results: [] });
    }, [restartApi]);

    /**
     * Close bulk action modal and reset state
     */
    const closeBulkAction = useCallback((): void => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    /**
     * Execute bulk action on items
     */
    const confirmBulkAction = useCallback(async (items: Resource[]): Promise<void> => {
        const action = bulkActionModal.action;
        const api = action === 'delete' ? deleteApi : restartApi;

        if (!api) {
            Logger.error(`No API configured for action: ${action}`, undefined, 'k8s');
            return;
        }

        Logger.info(`Bulk ${action} started`, { resourceType, count: items.length }, 'k8s');
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));

        const results: BulkActionResult[] = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            if (!name) {
                Logger.error('Item missing name in metadata', { item }, 'k8s');
                continue;
            }

            try {
                if (isNamespaced) {
                    await (api as NamespacedApiFn)(namespace || '', name);
                } else {
                    await (api as ClusterScopedApiFn)(name);
                }
                results.push({ name, namespace: namespace || '', success: true, message: '' });
                Logger.info(`${resourceLabel} ${action}d`, { namespace, name }, 'k8s');
            } catch (err: any) {
                const errorMessage = err instanceof Error ? err.message : String(err);
                results.push({ name, namespace: namespace || '', success: false, message: errorMessage });
                Logger.error(`Failed to ${action} ${resourceLabel.toLowerCase()}`, { namespace, name, error: err }, 'k8s');
            }

            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }

        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
        Logger.info(`Bulk ${action} completed`, {
            resourceType,
            total: items.length,
            success: results.filter((r: any) => r.success).length,
            failed: results.filter((r: any) => !r.success).length,
        }, 'k8s');
    }, [bulkActionModal.action, deleteApi, restartApi, isNamespaced, resourceLabel, resourceType]);

    /**
     * Export YAML backup for items
     */
    const exportYaml = useCallback(async (items: Resource[], options: ExportYamlOptions = {}): Promise<void> => {
        const { onProgress, signal } = options;
        Logger.info('Exporting YAML backup', { resourceType, count: items.length }, 'k8s');

        const entries: main.YamlBackupEntry[] = [];
        for (let i = 0; i < items.length; i++) {
            if (signal?.aborted) break;

            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            if (!name) {
                Logger.error('Item missing name in metadata', { item }, 'k8s');
                continue;
            }

            try {
                let yaml: string;
                if (isNamespaced) {
                    yaml = await (getYamlApi as (ns: string, n: string) => Promise<string>)(namespace || '', name);
                } else {
                    yaml = await (getYamlApi as (n: string) => Promise<string>)(name);
                }
                const entry = new main.YamlBackupEntry({
                    namespace: namespace || '',
                    name,
                    kind: resourceLabel,
                    yaml
                });
                entries.push(entry);
                Logger.info('Fetched YAML for backup', { namespace, name }, 'k8s');
            } catch (err: any) {
                const errorMessage = err instanceof Error ? err.message : String(err);
                Logger.error('Failed to get YAML for backup', { namespace, name, error: err }, 'k8s');
                const entry = new main.YamlBackupEntry({
                    namespace: namespace || '',
                    name,
                    kind: resourceLabel,
                    yaml: `# Failed to fetch YAML: ${errorMessage}`
                });
                entries.push(entry);
            }

            onProgress?.(i + 1, items.length);
        }

        if (entries.length === 0) return;

        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        const filename = `${resourceType}-backup-${timestamp}.zip`;

        try {
            await SaveYamlBackup(entries, filename);
            Logger.info('YAML backup saved', { filename, partial: signal?.aborted }, 'k8s');
        } catch (err: any) {
            Logger.error('Failed to save YAML backup', { error: err }, 'k8s');
            if (err) {
                const errorMessage = err instanceof Error ? err.message : String(err);
                if (errorMessage !== '') {
                    Logger.error('Backup save failed', { error: errorMessage }, 'k8s');
                }
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
