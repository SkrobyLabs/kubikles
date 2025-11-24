import React, { useState, useEffect, useRef } from 'react';
import Editor from '@monaco-editor/react';
import { GetSecretYaml, UpdateSecretYaml, GetSecretData, UpdateSecretData } from '../../../wailsjs/go/main/App';
import Logger from '../../utils/Logger';
import { EyeIcon, EyeSlashIcon, TrashIcon, PlusIcon } from '@heroicons/react/24/outline';

const MODE_YAML = 'yaml';
const MODE_KEYVALUE = 'keyvalue';

// Convert object to array with stable IDs
const objectToEntries = (obj) => {
    return Object.entries(obj || {}).map(([key, value], index) => ({
        id: `entry-${Date.now()}-${index}`,
        key,
        value
    }));
};

// Convert array back to object for saving
const entriesToObject = (entries) => {
    const result = {};
    entries.forEach(({ key, value }) => {
        if (key) result[key] = value;
    });
    return result;
};

export default function SecretEditor({ namespace, resourceName, onClose }) {
    const [mode, setMode] = useState(MODE_YAML);
    const [yamlContent, setYamlContent] = useState('');
    const [secretEntries, setSecretEntries] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [saving, setSaving] = useState(false);
    const [showBase64, setShowBase64] = useState(true);
    const editorRef = useRef(null);
    const nextIdRef = useRef(0);

    const generateId = () => {
        nextIdRef.current += 1;
        return `entry-${nextIdRef.current}`;
    };

    useEffect(() => {
        fetchData();
    }, [namespace, resourceName]);

    const fetchData = async () => {
        setLoading(true);
        setError(null);
        Logger.debug("Fetching secret data...", { namespace, name: resourceName });
        try {
            const [yaml, data] = await Promise.all([
                GetSecretYaml(namespace, resourceName),
                GetSecretData(namespace, resourceName)
            ]);
            setYamlContent(yaml);
            setSecretEntries(objectToEntries(data));
            Logger.info("Secret data fetched successfully", { namespace, name: resourceName });
        } catch (err) {
            Logger.error("Failed to load secret", err);
            setError(`Failed to load secret: ${err}`);
        } finally {
            setLoading(false);
        }
    };

    const handleSaveYaml = async () => {
        setSaving(true);
        Logger.info("Saving YAML...", { namespace, name: resourceName });
        try {
            await UpdateSecretYaml(namespace, resourceName, yamlContent);
            Logger.info("YAML saved successfully", { namespace, name: resourceName });
            alert("Secret saved successfully!");
            // Refresh key-value data after YAML save
            const data = await GetSecretData(namespace, resourceName);
            setSecretEntries(objectToEntries(data));
        } catch (err) {
            Logger.error("Failed to save secret", err);
            alert(`Failed to save secret: ${err}`);
        } finally {
            setSaving(false);
        }
    };

    const handleSaveKeyValue = async () => {
        setSaving(true);
        Logger.info("Saving secret data...", { namespace, name: resourceName });
        try {
            const dataToSave = entriesToObject(secretEntries);
            await UpdateSecretData(namespace, resourceName, dataToSave);
            Logger.info("Secret data saved successfully", { namespace, name: resourceName });
            alert("Secret saved successfully!");
            // Refresh YAML after key-value save
            const yaml = await GetSecretYaml(namespace, resourceName);
            setYamlContent(yaml);
        } catch (err) {
            Logger.error("Failed to save secret", err);
            alert(`Failed to save secret: ${err}`);
        } finally {
            setSaving(false);
        }
    };

    const handleSave = () => {
        if (mode === MODE_YAML) {
            handleSaveYaml();
        } else {
            handleSaveKeyValue();
        }
    };

    const handleEditorDidMount = (editor, monaco) => {
        editorRef.current = editor;
        editor.updateOptions({
            minimap: { enabled: true },
            scrollBeyondLastLine: false,
            fontSize: 13,
            lineNumbers: 'on',
            folding: true,
            renderWhitespace: 'selection',
            wordWrap: 'off',
        });
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
            handleSave();
        });
    };

    const handleKeyChange = (id, newKey) => {
        setSecretEntries(entries =>
            entries.map(entry =>
                entry.id === id ? { ...entry, key: newKey } : entry
            )
        );
    };

    const handleValueChange = (id, newValue) => {
        setSecretEntries(entries =>
            entries.map(entry =>
                entry.id === id ? { ...entry, value: newValue } : entry
            )
        );
    };

    const handleDeleteEntry = (id) => {
        setSecretEntries(entries => entries.filter(entry => entry.id !== id));
    };

    const handleAddKey = () => {
        const existingKeys = new Set(secretEntries.map(e => e.key));
        let newKey = 'NEW_KEY';
        let counter = 1;
        while (existingKeys.has(newKey)) {
            newKey = `NEW_KEY_${counter}`;
            counter++;
        }
        setSecretEntries([...secretEntries, { id: generateId(), key: newKey, value: '' }]);
    };

    const encodeBase64 = (str) => {
        try {
            return btoa(str);
        } catch {
            // Handle binary data that can't be encoded with btoa
            return btoa(unescape(encodeURIComponent(str)));
        }
    };

    const decodeBase64 = (str) => {
        try {
            return atob(str);
        } catch {
            return str;
        }
    };

    const getDisplayValue = (value) => {
        if (showBase64) {
            return encodeBase64(value);
        }
        return value;
    };

    const setValueFromDisplay = (id, displayValue) => {
        if (showBase64) {
            handleValueChange(id, decodeBase64(displayValue));
        } else {
            handleValueChange(id, displayValue);
        }
    };

    if (loading) {
        return (
            <div className="flex items-center justify-center h-full text-gray-400">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mr-2"></div>
                Loading secret...
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
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {namespace}/{resourceName}
                    </div>
                    {/* Mode Toggle */}
                    <div className="flex items-center bg-[#2d2d2d] rounded-md p-0.5">
                        <button
                            onClick={() => setMode(MODE_YAML)}
                            className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                mode === MODE_YAML
                                    ? 'bg-primary text-white'
                                    : 'text-gray-400 hover:text-white'
                            }`}
                        >
                            YAML
                        </button>
                        <button
                            onClick={() => setMode(MODE_KEYVALUE)}
                            className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                mode === MODE_KEYVALUE
                                    ? 'bg-primary text-white'
                                    : 'text-gray-400 hover:text-white'
                            }`}
                        >
                            Key-Value
                        </button>
                    </div>
                    {/* Base64 Toggle - only visible in Key-Value mode */}
                    {mode === MODE_KEYVALUE && (
                        <button
                            onClick={() => setShowBase64(!showBase64)}
                            className={`flex items-center gap-1.5 px-2 py-1 text-xs font-medium rounded transition-colors ${
                                showBase64
                                    ? 'bg-amber-600/20 text-amber-400 hover:bg-amber-600/30'
                                    : 'bg-green-600/20 text-green-400 hover:bg-green-600/30'
                            }`}
                            title={showBase64 ? 'Values shown as Base64' : 'Values shown decoded'}
                        >
                            {showBase64 ? (
                                <>
                                    <EyeSlashIcon className="h-3.5 w-3.5" />
                                    Base64
                                </>
                            ) : (
                                <>
                                    <EyeIcon className="h-3.5 w-3.5" />
                                    Decoded
                                </>
                            )}
                        </button>
                    )}
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

            {/* Content Area */}
            <div className="flex-1 overflow-hidden">
                {mode === MODE_YAML ? (
                    <Editor
                        height="100%"
                        defaultLanguage="yaml"
                        value={yamlContent}
                        onChange={(value) => setYamlContent(value || '')}
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
                ) : (
                    <div className="h-full overflow-auto p-4">
                        <div className="space-y-2">
                            {secretEntries.map((entry) => (
                                <div key={entry.id} className="flex items-start gap-2 bg-[#2d2d2d] rounded-md p-3">
                                    <input
                                        type="text"
                                        value={entry.key}
                                        onChange={(e) => handleKeyChange(entry.id, e.target.value)}
                                        className="w-48 shrink-0 px-2 py-1.5 text-sm bg-[#1e1e1e] border border-[#3d3d3d] rounded text-gray-200 focus:outline-none focus:border-primary"
                                        placeholder="Key"
                                        autoComplete="off"
                                        autoCorrect="off"
                                        autoCapitalize="off"
                                        spellCheck="false"
                                    />
                                    <textarea
                                        value={getDisplayValue(entry.value)}
                                        onChange={(e) => setValueFromDisplay(entry.id, e.target.value)}
                                        className="flex-1 min-h-[60px] px-2 py-1.5 text-sm bg-[#1e1e1e] border border-[#3d3d3d] rounded text-gray-200 font-mono focus:outline-none focus:border-primary resize-y"
                                        placeholder="Value"
                                        autoComplete="off"
                                        autoCorrect="off"
                                        autoCapitalize="off"
                                        spellCheck="false"
                                    />
                                    <button
                                        onClick={() => handleDeleteEntry(entry.id)}
                                        className="p-1.5 text-red-400 hover:text-red-300 hover:bg-red-900/20 rounded transition-colors"
                                        title="Delete key"
                                    >
                                        <TrashIcon className="h-4 w-4" />
                                    </button>
                                </div>
                            ))}
                            {secretEntries.length === 0 && (
                                <div className="text-gray-500 text-sm text-center py-8">
                                    No secret data. Click "Add Key" to create one.
                                </div>
                            )}
                        </div>
                        <button
                            onClick={handleAddKey}
                            className="mt-4 flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-400 hover:text-white bg-[#2d2d2d] hover:bg-[#3d3d3d] rounded transition-colors"
                        >
                            <PlusIcon className="h-4 w-4" />
                            Add Key
                        </button>
                    </div>
                )}
            </div>
        </div>
    );
}
