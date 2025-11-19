import React, { useState, useEffect } from 'react';
import { GetPodYaml, UpdatePodYaml } from '../../wailsjs/go/main/App';

export default function YamlEditor({ namespace, podName, onClose }) {
    const [content, setContent] = useState('');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        fetchYaml();
    }, [namespace, podName]);

    const fetchYaml = async () => {
        setLoading(true);
        setError(null);
        try {
            const yaml = await GetPodYaml(namespace, podName);
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
            await UpdatePodYaml(namespace, podName, content);
            // Optionally show success message or just close/refresh?
            // For now, we'll just stay open to allow further edits, or maybe close?
            // The user requirement implies a "Save" button, usually implies saving and staying or saving and closing.
            // Given it's a tab, saving and staying seems appropriate, but maybe we should give feedback.
            // Let's just save for now.
            alert("YAML saved successfully!");
        } catch (err) {
            alert(`Failed to save YAML: ${err}`);
        } finally {
            setSaving(false);
        }
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

            {/* Editor Area */}
            <div className="flex-1 overflow-hidden">
                <textarea
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                    className="w-full h-full bg-[#1e1e1e] text-gray-300 font-mono text-sm p-4 resize-none focus:outline-none"
                    spellCheck="false"
                />
            </div>
        </div>
    );
}
