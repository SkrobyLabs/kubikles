import React, { useState, useEffect, useRef } from 'react';
import Editor from '@monaco-editor/react';
import { GetHelmReleaseAllValues, GetHelmReleaseValues } from '../../../../wailsjs/go/main/App';
import { useK8s } from '../../../context/K8sContext';
import { ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import Logger from '../../../utils/Logger';
import yaml from 'js-yaml';

export default function HelmReleaseValuesTab({ release, isStale }) {
    const { currentContext, lastRefresh } = useK8s();
    const [content, setContent] = useState('');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [format, setFormat] = useState('yaml'); // 'yaml' or 'json'
    const [showUserOnly, setShowUserOnly] = useState(false);
    const [rawValues, setRawValues] = useState(null);
    const editorRef = useRef(null);

    // Fetch values
    useEffect(() => {
        if (!currentContext || !release || isStale) {
            setError(isStale ? 'This tab was opened in a different context' : null);
            setLoading(false);
            return;
        }

        const fetchValues = async () => {
            setLoading(true);
            setError(null);
            try {
                Logger.info("Fetching Helm release values", { namespace: release.namespace, name: release.name, userOnly: showUserOnly });
                const values = showUserOnly
                    ? await GetHelmReleaseValues(release.namespace, release.name)
                    : await GetHelmReleaseAllValues(release.namespace, release.name);
                setRawValues(values || {});
            } catch (err) {
                Logger.error("Failed to fetch Helm release values", err);
                setError(err.message || String(err));
            } finally {
                setLoading(false);
            }
        };

        fetchValues();
    }, [currentContext, release, isStale, showUserOnly, lastRefresh]);

    // Format content when raw values or format changes
    useEffect(() => {
        if (!rawValues) return;

        try {
            if (format === 'yaml') {
                setContent(yaml.dump(rawValues, {
                    indent: 2,
                    lineWidth: -1,
                    noRefs: true,
                    sortKeys: true
                }));
            } else {
                setContent(JSON.stringify(rawValues, null, 2));
            }
        } catch (err) {
            Logger.error("Failed to format values", err);
            setContent(JSON.stringify(rawValues, null, 2));
        }
    }, [rawValues, format]);

    const handleEditorDidMount = (editor) => {
        editorRef.current = editor;
    };

    if (loading) {
        return (
            <div className="flex items-center justify-center h-full bg-[#1e1e1e]">
                <div className="flex items-center gap-3 text-gray-400">
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                    <span>Loading values...</span>
                </div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex flex-col items-center justify-center h-full bg-[#1e1e1e] text-red-400 p-4">
                <ExclamationTriangleIcon className="h-8 w-8 mb-2" />
                <span className="text-center">{error}</span>
            </div>
        );
    }

    return (
        <div className="h-full flex flex-col bg-[#1e1e1e]">
            {/* Toolbar */}
            <div className="flex items-center justify-between px-4 py-2 bg-[#252526] border-b border-[#3c3c3c]">
                <div className="flex items-center gap-4">
                    {/* User Values Only Toggle */}
                    <label className="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
                        <input
                            type="checkbox"
                            checked={showUserOnly}
                            onChange={(e) => setShowUserOnly(e.target.checked)}
                            className="w-4 h-4 rounded border-gray-600 bg-gray-700 text-primary focus:ring-primary focus:ring-offset-0"
                        />
                        User values only
                    </label>
                </div>

                <div className="flex items-center gap-2">
                    {/* Format Toggle */}
                    <div className="flex items-center bg-[#2d2d2d] rounded p-0.5">
                        <button
                            onClick={() => setFormat('yaml')}
                            className={`px-2 py-1 text-xs font-medium rounded transition-colors ${
                                format === 'yaml'
                                    ? 'bg-primary text-white'
                                    : 'text-gray-400 hover:text-white'
                            }`}
                        >
                            YAML
                        </button>
                        <button
                            onClick={() => setFormat('json')}
                            className={`px-2 py-1 text-xs font-medium rounded transition-colors ${
                                format === 'json'
                                    ? 'bg-primary text-white'
                                    : 'text-gray-400 hover:text-white'
                            }`}
                        >
                            JSON
                        </button>
                    </div>
                    <span className="text-xs text-gray-500 bg-gray-700 px-2 py-0.5 rounded">
                        Read Only
                    </span>
                </div>
            </div>

            {/* Empty state for user values */}
            {showUserOnly && rawValues && Object.keys(rawValues).length === 0 && (
                <div className="flex items-center justify-center h-full text-gray-500">
                    No user-provided values for this release
                </div>
            )}

            {/* Editor */}
            {(!showUserOnly || Object.keys(rawValues || {}).length > 0) && (
                <div className="flex-1 overflow-hidden">
                    <Editor
                        height="100%"
                        defaultLanguage={format}
                        language={format}
                        value={content}
                        theme="vs-dark"
                        onMount={handleEditorDidMount}
                        options={{
                            readOnly: true,
                            minimap: { enabled: false },
                            scrollBeyondLastLine: false,
                            fontSize: 13,
                            lineNumbers: 'on',
                            renderLineHighlight: 'line',
                            automaticLayout: true,
                            wordWrap: 'on',
                            folding: true,
                            foldingStrategy: 'indentation'
                        }}
                    />
                </div>
            )}
        </div>
    );
}
