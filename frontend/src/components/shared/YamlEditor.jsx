import React, { useState, useEffect, useRef } from 'react';
import Editor from '@monaco-editor/react';
import {
    GetPodYaml, UpdatePodYaml,
    GetDeploymentYaml, UpdateDeploymentYaml,
    GetStatefulSetYaml, UpdateStatefulSetYaml,
    GetConfigMapYaml, UpdateConfigMapYaml,
    GetSecretYaml, UpdateSecretYaml,
    GetDaemonSetYaml, UpdateDaemonSetYaml,
    GetReplicaSetYaml, UpdateReplicaSetYaml
} from '../../../wailsjs/go/main/App';

export default function YamlEditor({ namespace, podName, isDeployment, isStatefulSet, isConfigMap, isSecret, isDaemonSet, isReplicaSet, onClose }) {
    const [content, setContent] = useState('');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [saving, setSaving] = useState(false);
    const editorRef = useRef(null);

    useEffect(() => {
        fetchYaml();
    }, [namespace, podName]);

    const fetchYaml = async () => {
        setLoading(true);
        setError(null);
        try {
            let yaml;
            if (isDeployment) {
                yaml = await GetDeploymentYaml(namespace, podName);
            } else if (isStatefulSet) {
                yaml = await GetStatefulSetYaml(namespace, podName);
            } else if (isConfigMap) {
                yaml = await GetConfigMapYaml(namespace, podName);
            } else if (isSecret) {
                yaml = await GetSecretYaml(namespace, podName);
            } else if (isDaemonSet) {
                yaml = await GetDaemonSetYaml(namespace, podName);
            } else if (isReplicaSet) {
                yaml = await GetReplicaSetYaml(namespace, podName);
            } else {
                yaml = await GetPodYaml(namespace, podName);
            }
            setContent(yaml);
        } catch (err) {
            setError(`Failed to load YAML: ${err}`);
        } finally {
            setLoading(false);
        }
    };

    const handleSave = async () => {
        setSaving(true);
        try {
            if (isDeployment) {
                await UpdateDeploymentYaml(namespace, podName, content);
            } else if (isStatefulSet) {
                await UpdateStatefulSetYaml(namespace, podName, content);
            } else if (isConfigMap) {
                await UpdateConfigMapYaml(namespace, podName, content);
            } else if (isSecret) {
                await UpdateSecretYaml(namespace, podName, content);
            } else if (isDaemonSet) {
                await UpdateDaemonSetYaml(namespace, podName, content);
            } else if (isReplicaSet) {
                await UpdateReplicaSetYaml(namespace, podName, content);
            } else {
                await UpdatePodYaml(namespace, podName, content);
            }
            alert("YAML saved successfully!");
        } catch (err) {
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
                    {namespace}/{podName}
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
