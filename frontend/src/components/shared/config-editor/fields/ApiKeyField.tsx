import React, { useState, useEffect, useCallback } from 'react';
import { GetAnthropicAPIKeyStatus, SetAnthropicAPIKey, ClearAnthropicAPIKey } from '~/lib/wailsjs-adapter/go/main/App';

interface Props {
    label: string;
    description?: string;
}

export default function ApiKeyField({ label, description }: Props) {
    const [status, setStatus] = useState<string>('');
    const [inputValue, setInputValue] = useState('');
    const [isEditing, setIsEditing] = useState(false);
    const [saving, setSaving] = useState(false);

    const loadStatus = useCallback(() => {
        GetAnthropicAPIKeyStatus().then(setStatus).catch(() => setStatus('not_set'));
    }, []);

    useEffect(() => {
        loadStatus();
    }, [loadStatus]);

    const handleSave = async () => {
        const trimmed = inputValue.trim();
        if (!trimmed) return;
        setSaving(true);
        try {
            await SetAnthropicAPIKey(trimmed);
            setInputValue('');
            setIsEditing(false);
            loadStatus();
        } catch (err) {
            console.error('Failed to save API key:', err);
        } finally {
            setSaving(false);
        }
    };

    const handleClear = async () => {
        try {
            await ClearAnthropicAPIKey();
            loadStatus();
        } catch (err) {
            console.error('Failed to clear API key:', err);
        }
    };

    const statusLabel = status === 'env'
        ? 'Set via environment variable'
        : status === 'configured'
            ? 'Configured'
            : 'Not configured';

    const statusColor = status === 'not_set'
        ? 'text-yellow-500'
        : 'text-green-500';

    return (
        <div className="py-2">
            <div className="text-sm font-medium text-text mb-1">{label}</div>
            {description && (
                <p className="text-xs text-text-muted mb-2">{description}</p>
            )}
            <div className="flex items-center gap-3">
                <span className={`text-xs font-medium ${statusColor}`}>
                    {statusLabel}
                </span>
                {status === 'env' ? (
                    <span className="text-xs text-text-muted">(managed externally)</span>
                ) : isEditing ? (
                    <div className="flex items-center gap-1.5">
                        <input
                            type="password"
                            value={inputValue}
                            onChange={e => setInputValue(e.target.value)}
                            onKeyDown={e => {
                                if (e.key === 'Enter') handleSave();
                                if (e.key === 'Escape') { setIsEditing(false); setInputValue(''); }
                            }}
                            placeholder="sk-ant-..."
                            className="px-2 py-1 text-sm bg-background border border-border rounded text-text focus:outline-none focus:border-primary w-64"
                            autoFocus
                        />
                        <button
                            onClick={handleSave}
                            disabled={!inputValue.trim() || saving}
                            className="px-2 py-1 text-xs bg-primary text-white rounded disabled:opacity-40"
                        >
                            {saving ? '...' : 'Save'}
                        </button>
                        <button
                            onClick={() => { setIsEditing(false); setInputValue(''); }}
                            className="px-2 py-1 text-xs text-text-muted hover:text-text"
                        >
                            Cancel
                        </button>
                    </div>
                ) : (
                    <div className="flex items-center gap-1.5">
                        <button
                            onClick={() => setIsEditing(true)}
                            className="px-2 py-1 text-xs text-primary hover:text-primary/80"
                        >
                            {status === 'configured' ? 'Change' : 'Set Key'}
                        </button>
                        {status === 'configured' && (
                            <button
                                onClick={handleClear}
                                className="px-2 py-1 text-xs text-red-400 hover:text-red-300"
                            >
                                Clear
                            </button>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}
