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

export default function YamlEditor({ namespace, resourceName, isDeployment, isStatefulSet, isConfigMap, isSecret, isDaemonSet, isReplicaSet, isJob, isCronJob, isNamespace, isEvent, onClose }) {
    const [content, setContent] = useState('');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [saving, setSaving] = useState(false);
    const editorRef = useRef(null);

    useEffect(() => {
        fetchYaml();
    }, [namespace, resourceName]);

    const fetchYaml = async () => {
        setLoading(true);
        setError(null);
        Logger.debug("Fetching YAML...", { namespace, name: resourceName });
        try {
            let yaml;
            if (isDeployment) {
                yaml = await GetDeploymentYaml(namespace, resourceName);
            } else if (isStatefulSet) {
                yaml = await GetStatefulSetYaml(namespace, resourceName);
            } else if (isConfigMap) {
                yaml = await GetConfigMapYaml(namespace, resourceName);
            } else if (isSecret) {
                yaml = await GetSecretYaml(namespace, resourceName);
            } else if (isDaemonSet) {
                yaml = await GetDaemonSetYaml(namespace, resourceName);
            } else if (isReplicaSet) {
                yaml = await GetReplicaSetYaml(namespace, resourceName);
            } else if (isJob) {
                yaml = await GetJobYaml(namespace, resourceName);
            } else if (isCronJob) {
                yaml = await GetCronJobYaml(namespace, resourceName);
            } else if (isNamespace) {
                yaml = await GetNamespaceYAML(resourceName);
            } else if (isEvent) {
                yaml = await GetEventYAML(namespace, resourceName);
            } else {
                yaml = await GetPodYaml(namespace, resourceName);
            }
            setContent(yaml);
            Logger.info("YAML fetched successfully", { namespace, name: resourceName });
        } catch (err) {
            Logger.error("Failed to load YAML", err);
            setError(`Failed to load YAML: ${err}`);
        } finally {
            setLoading(false);
        }
    };

    const handleSave = async () => {
        setSaving(true);
        Logger.info("Saving YAML...", { namespace, name: resourceName });
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
            alert("YAML saved successfully!");
        } catch (err) {
            Logger.error("Failed to save YAML", err);
            alert(`Failed to save YAML: ${err}`);
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

        // Add Cmd+S / Ctrl+S save shortcut
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
            handleSave();
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
                        disabled={saving}
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
