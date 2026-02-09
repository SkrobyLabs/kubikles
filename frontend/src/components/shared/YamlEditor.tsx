import React, { useState, useEffect, useRef, useMemo } from 'react';
import Editor from '@monaco-editor/react';
import { getResource, getResourceByKind } from '~/utils/resourceRegistry';
import Logger from '~/utils/Logger';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { ExclamationTriangleIcon, InformationCircleIcon, LockClosedIcon } from '@heroicons/react/24/outline';

// Extract controller owner from YAML content
function extractControllerOwner(yamlContent: any) {
    if (!yamlContent) return null;

    // Find ownerReferences section
    const ownerRefsMatch = yamlContent.match(/ownerReferences:\s*\n((?:\s+-[\s\S]*?(?=\n\S|\n\s*$))+)/);
    if (!ownerRefsMatch) return null;

    const ownerRefsBlock = ownerRefsMatch[1];

    // Find the controller owner (controller: true)
    const entries = ownerRefsBlock.split(/\n\s+-\s+/).filter(Boolean);

    for (const entry of entries) {
        if (entry.includes('controller: true') || entry.includes('controller:true')) {
            const kindMatch = entry.match(/kind:\s*(\S+)/);
            const nameMatch = entry.match(/name:\s*(\S+)/);
            const uidMatch = entry.match(/uid:\s*(\S+)/);

            if (kindMatch && nameMatch) {
                return {
                    kind: kindMatch[1],
                    name: nameMatch[1],
                    uid: uidMatch ? uidMatch[1] : null
                };
            }
        }
    }

    return null;
}

export default function YamlEditor({
    resourceType,
    namespace,
    resourceName,
    onClose,
    // Optional custom functions for custom resources (bypasses registry)
    getYamlFn,
    updateYamlFn,
    tabContext = ''
}: {
    resourceType: any;
    namespace: any;
    resourceName: any;
    onClose: any;
    getYamlFn?: any;
    updateYamlFn?: any;
    tabContext?: string;
}) {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();
    const [content, setContent] = useState('');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [saving, setSaving] = useState(false);
    const [hasConflict, setHasConflict] = useState(false);
    const editorRef = useRef<any>(null);

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    // Get resource definition from registry (only if custom functions not provided)
    const resource = useMemo(() => {
        if (getYamlFn && updateYamlFn) return null; // Custom functions provided, skip registry
        return getResource(resourceType);
    }, [resourceType, getYamlFn, updateYamlFn]);

    // Extract controller owner from content
    const controllerOwner = useMemo(() => extractControllerOwner(content), [content]);

    // Handle opening the controlling resource's edit dialog
    const handleEditOwner = () => {
        if (!controllerOwner) return;

        const ownerResource = getResourceByKind(controllerOwner.kind);
        if (!ownerResource) {
            Logger.warn("Unknown controller kind", { kind: controllerOwner.kind }, 'k8s');
            return;
        }

        const ownerType = controllerOwner.kind.toLowerCase();
        const tabId = `yaml-${ownerType}-${namespace}-${controllerOwner.name}`;

        openTab({
            id: tabId,
            title: `${controllerOwner.name}`,
            content: (
                <YamlEditor
                    resourceType={ownerType}
                    namespace={namespace}
                    resourceName={controllerOwner.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    // Helper: Get editor state for cursor restoration
    const getEditorState = () => {
        if (!editorRef.current) return null;
        return {
            position: editorRef.current.getPosition(),
            scrollTop: editorRef.current.getScrollTop()
        };
    };

    // Helper: Restore editor state after content refresh
    const restoreEditorState = (state: any) => {
        if (!editorRef.current || !state) return;
        const editor = editorRef.current;
        const model = editor.getModel();
        if (!model) return;

        // Clamp position to valid range
        const lineCount = model.getLineCount();
        const line = Math.min(state.position.lineNumber, lineCount);
        const maxCol = model.getLineMaxColumn(line);
        const col = Math.min(state.position.column, maxCol);

        editor.setPosition({ lineNumber: line, column: col });
        editor.setScrollTop(state.scrollTop);
        editor.focus();
    };

    // Fetch YAML using custom function or registry
    const getYaml = async () => {
        if (getYamlFn) {
            return getYamlFn();
        }
        if (!resource) {
            throw new Error(`Unknown resource type: ${resourceType}`);
        }
        return resource.getYaml(namespace, resourceName);
    };

    useEffect(() => {
        fetchYaml();
    }, [namespace, resourceName, resourceType]);

    const fetchYaml = async () => {
        setLoading(true);
        setError(null);
        setHasConflict(false);
        Logger.debug("Fetching YAML...", { resourceType, namespace, name: resourceName }, 'k8s');
        try {
            const yaml = await getYaml();
            setContent(yaml);
            Logger.info("YAML fetched successfully", { resourceType, namespace, name: resourceName }, 'k8s');
        } catch (err: any) {
            Logger.error("Failed to load YAML", err, 'k8s');
            setError(`Failed to load YAML: ${err}`);
        } finally {
            setLoading(false);
        }
    };

    // Reload YAML (used when conflict detected)
    const handleReload = async () => {
        try {
            const yaml = await getYaml();
            setContent(yaml);
            setHasConflict(false);
            Logger.info("YAML reloaded", { resourceType, namespace, name: resourceName }, 'k8s');
        } catch (err: any) {
            Logger.error("Failed to reload YAML", err, 'k8s');
            addNotification({ type: 'error', title: 'Failed to reload YAML', message: String(err) });
        }
    };

    const handleSave = async () => {
        if (!resource && !updateYamlFn) {
            addNotification({ type: 'error', title: 'Unknown resource type', message: resourceType });
            return;
        }

        setSaving(true);
        Logger.info("Saving YAML...", { resourceType, namespace, name: resourceName }, 'k8s');

        // Save editor state for cursor restoration
        const savedState = getEditorState();

        try {
            if (updateYamlFn) {
                await updateYamlFn(content);
            } else {
                await resource!.updateYaml(namespace, resourceName, content);
            }

            Logger.info("YAML saved successfully", { resourceType, namespace, name: resourceName }, 'k8s');

            // Refresh YAML to get updated resourceVersion
            try {
                const yaml = await getYaml();
                setContent(yaml);
                setHasConflict(false);

                // Restore cursor position after content update
                requestAnimationFrame(() => restoreEditorState(savedState));
            } catch (refreshErr) {
                Logger.warn("Failed to refresh YAML after save", refreshErr, 'k8s');
            }

            addNotification({ type: 'success', title: 'YAML saved successfully', message: '' });
        } catch (err: any) {
            Logger.error("Failed to save YAML", err, 'k8s');

            // Check for 409 Conflict (stale resourceVersion)
            const errStr = (err as any).toString().toLowerCase();
            if (errStr.includes('409') || errStr.includes('conflict') || errStr.includes('modified')) {
                setHasConflict(true);
                Logger.warn("Conflict detected - resource was modified externally", { resourceType, namespace, name: resourceName }, 'k8s');
            } else {
                addNotification({ type: 'error', title: 'Failed to save YAML', message: String(err) });
            }
        } finally {
            setSaving(false);
        }
    };

    const handleEditorDidMount = (editor: any, monaco: any) => {
        editorRef.current = editor;

        // Configure editor
        editor.updateOptions({
            minimap: { enabled: true },
            scrollBeyondLastLine: false,
            fontSize: 13,
            lineNumbers: 'on',
            folding: true,
            renderWhitespace: 'selection',
            wordWrap: 'off',
        });

        // Add Cmd+S / Ctrl+S save shortcut (disabled during conflict or when stale)
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
            if (!hasConflict && !isStale) {
                handleSave();
            }
        });
    };

    if (loading) {
        return (
            <div className="flex items-center justify-center h-full text-gray-400">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mr-2"></div>
                Loading YAML...
            </div>
        );
    }

    if (error) {
        return (
            <div className="p-4 text-red-400">
                {error}
            </div>
        );
    }

    // For custom resources or resources from registry
    const isNamespaced = namespace !== undefined && namespace !== '';

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <LockClosedIcon className="h-5 w-5" />
                    <span className="text-sm">
                        Read-only: This YAML is from context <span className="font-medium">{tabContext}</span>. Switch back to edit.
                    </span>
                </div>
            )}

            {/* Conflict Warning Banner */}
            {hasConflict && !isStale && (
                <div className="flex items-center justify-between px-4 py-2 bg-yellow-500/20 border-b border-yellow-500 text-yellow-400 shrink-0">
                    <div className="flex items-center gap-2">
                        <ExclamationTriangleIcon className="h-5 w-5" />
                        <span className="text-sm font-medium">
                            Resource was modified externally. Your changes conflict with the server version.
                        </span>
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            onClick={handleReload}
                            className="px-3 py-1 text-xs font-medium bg-yellow-500/30 hover:bg-yellow-500/40 rounded transition-colors"
                        >
                            Reload
                        </button>
                        <button
                            onClick={handleSave}
                            disabled={saving}
                            className="px-3 py-1 text-xs font-medium bg-red-500/30 hover:bg-red-500/40 text-red-400 rounded transition-colors disabled:opacity-50"
                        >
                            {saving ? 'Saving...' : 'Force Save'}
                        </button>
                    </div>
                </div>
            )}

            {/* Controller Owner Info Banner */}
            {controllerOwner && (
                <div className="flex items-center justify-between px-4 py-2 bg-blue-500/20 border-b border-blue-500/50 text-blue-400 shrink-0">
                    <div className="flex items-center gap-2">
                        <InformationCircleIcon className="h-5 w-5" />
                        <span className="text-sm">
                            This resource is controlled by <span className="font-medium">{controllerOwner.kind}/{controllerOwner.name}</span>. Changes may be overwritten.
                        </span>
                    </div>
                    <button
                        onClick={handleEditOwner}
                        className="px-3 py-1 text-xs font-medium bg-blue-500/30 hover:bg-blue-500/40 rounded transition-colors"
                    >
                        Edit {controllerOwner.kind}
                    </button>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="text-sm font-medium text-gray-400">
                    {isNamespaced ? `${namespace}/${resourceName}` : resourceName}
                </div>
                <div className="flex items-center gap-2">
                    <button
                        onClick={onClose}
                        className="px-3 py-1.5 text-xs font-medium text-gray-400 hover:text-text hover:bg-white/5 rounded transition-colors"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={handleSave}
                        disabled={saving || hasConflict || !!isStale}
                        title={isStale ? "Cannot save - tab is from a different context" : hasConflict ? "Resolve conflict using the options above" : "Save changes"}
                        className="px-3 py-1.5 text-xs font-medium bg-primary text-white rounded hover:bg-blue-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                        {saving ? 'Saving...' : 'Save'}
                    </button>
                </div>
            </div>

            {/* Monaco Editor */}
            <div className="flex-1 overflow-hidden">
                <Editor
                    height="100%"
                    defaultLanguage="yaml"
                    value={content}
                    onChange={(value: any) => !isStale && setContent(value || '')}
                    onMount={handleEditorDidMount}
                    theme="vs-dark"
                    options={{
                        automaticLayout: true,
                        readOnly: !!isStale,
                        scrollbar: {
                            vertical: 'auto',
                            horizontal: 'auto',
                        },
                    }}
                />
            </div>
        </div>
    );
}
