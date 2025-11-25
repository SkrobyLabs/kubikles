import React, { useState, useEffect, useRef } from 'react';
import Editor from '@monaco-editor/react';
import {
    GetPodYaml, UpdatePodYaml,
    GetDeploymentYaml, UpdateDeploymentYaml,
    GetStatefulSetYaml, UpdateStatefulSetYaml,
    GetConfigMapYaml, UpdateConfigMapYaml,
    GetSecretYaml, UpdateSecretYaml,
    GetDaemonSetYaml, UpdateDaemonSetYaml,
    GetReplicaSetYaml, UpdateReplicaSetYaml,
    GetJobYaml, UpdateJobYaml,
    GetCronJobYaml, UpdateCronJobYaml,
    GetNamespaceYAML, UpdateNamespaceYAML,
    GetEventYAML, UpdateEventYAML
} from '../../../wailsjs/go/main/App';
import Logger from '../../utils/Logger';
import { ExclamationTriangleIcon } from '@heroicons/react/24/outline';

export default function YamlEditor({ namespace, resourceName, isDeployment, isStatefulSet, isConfigMap, isSecret, isDaemonSet, isReplicaSet, isJob, isCronJob, isNamespace, isEvent, onClose }) {
    const [content, setContent] = useState('');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [saving, setSaving] = useState(false);
    const [hasConflict, setHasConflict] = useState(false);
    const editorRef = useRef(null);

    // Helper: Get editor state for cursor restoration
    const getEditorState = () => {
        if (!editorRef.current) return null;
        return {
            position: editorRef.current.getPosition(),
            scrollTop: editorRef.current.getScrollTop()
        };
    };

    // Helper: Restore editor state after content refresh
    const restoreEditorState = (state) => {
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

    // Helper: Get YAML for current resource type
    const getYaml = async () => {
        if (isDeployment) return GetDeploymentYaml(namespace, resourceName);
        if (isStatefulSet) return GetStatefulSetYaml(namespace, resourceName);
        if (isConfigMap) return GetConfigMapYaml(namespace, resourceName);
        if (isSecret) return GetSecretYaml(namespace, resourceName);
        if (isDaemonSet) return GetDaemonSetYaml(namespace, resourceName);
        if (isReplicaSet) return GetReplicaSetYaml(namespace, resourceName);
        if (isJob) return GetJobYaml(namespace, resourceName);
        if (isCronJob) return GetCronJobYaml(namespace, resourceName);
        if (isNamespace) return GetNamespaceYAML(resourceName);
        if (isEvent) return GetEventYAML(namespace, resourceName);
        return GetPodYaml(namespace, resourceName);
    };

    useEffect(() => {
        fetchYaml();
    }, [namespace, resourceName]);

    const fetchYaml = async () => {
        setLoading(true);
        setError(null);
        setHasConflict(false);
        Logger.debug("Fetching YAML...", { namespace, name: resourceName });
        try {
            const yaml = await getYaml();
            setContent(yaml);
            Logger.info("YAML fetched successfully", { namespace, name: resourceName });
        } catch (err) {
            Logger.error("Failed to load YAML", err);
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
            Logger.info("YAML reloaded", { namespace, name: resourceName });
        } catch (err) {
            Logger.error("Failed to reload YAML", err);
            alert(`Failed to reload YAML: ${err}`);
        }
    };

    const handleSave = async () => {
        setSaving(true);
        Logger.info("Saving YAML...", { namespace, name: resourceName });

        // Save editor state for cursor restoration
        const savedState = getEditorState();

        try {
            if (isDeployment) {
                await UpdateDeploymentYaml(namespace, resourceName, content);
            } else if (isStatefulSet) {
                await UpdateStatefulSetYaml(namespace, resourceName, content);
            } else if (isConfigMap) {
                await UpdateConfigMapYaml(namespace, resourceName, content);
            } else if (isSecret) {
                await UpdateSecretYaml(namespace, resourceName, content);
            } else if (isDaemonSet) {
                await UpdateDaemonSetYaml(namespace, resourceName, content);
            } else if (isReplicaSet) {
                await UpdateReplicaSetYaml(namespace, resourceName, content);
            } else if (isJob) {
                await UpdateJobYaml(namespace, resourceName, content);
            } else if (isCronJob) {
                await UpdateCronJobYaml(namespace, resourceName, content);
            } else if (isNamespace) {
                await UpdateNamespaceYAML(resourceName, content);
            } else if (isEvent) {
                await UpdateEventYAML(namespace, resourceName, content);
            } else {
                await UpdatePodYaml(namespace, resourceName, content);
            }

            Logger.info("YAML saved successfully", { namespace, name: resourceName });

            // Refresh YAML to get updated resourceVersion
            try {
                const yaml = await getYaml();
                setContent(yaml);
                setHasConflict(false);

                // Restore cursor position after content update
                requestAnimationFrame(() => restoreEditorState(savedState));
            } catch (refreshErr) {
                Logger.warn("Failed to refresh YAML after save", refreshErr);
            }

            alert("YAML saved successfully!");
        } catch (err) {
            Logger.error("Failed to save YAML", err);

            // Check for 409 Conflict (stale resourceVersion)
            const errStr = err.toString().toLowerCase();
            if (errStr.includes('409') || errStr.includes('conflict') || errStr.includes('modified')) {
                setHasConflict(true);
                Logger.warn("Conflict detected - resource was modified externally", { namespace, name: resourceName });
            } else {
                alert(`Failed to save YAML: ${err}`);
            }
        } finally {
            setSaving(false);
        }
    };

    const handleEditorDidMount = (editor, monaco) => {
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

        // Add Cmd+S / Ctrl+S save shortcut (disabled during conflict)
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
            if (!hasConflict) {
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

    return (
        <div className="flex flex-col h-full bg-[#1e1e1e]">
            {/* Conflict Warning Banner */}
            {hasConflict && (
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

            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="text-sm font-medium text-gray-400">
                    {isNamespace ? resourceName : `${namespace}/${resourceName}`}
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
                        disabled={saving || hasConflict}
                        title={hasConflict ? "Resolve conflict using the options above" : "Save changes"}
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
                    onChange={(value) => setContent(value || '')}
                    onMount={handleEditorDidMount}
                    theme="vs-dark"
                    options={{
                        automaticLayout: true,
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
