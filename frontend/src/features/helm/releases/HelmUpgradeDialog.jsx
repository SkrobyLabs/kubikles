import React, { useState, useCallback, useEffect, useRef } from 'react';
import { XMarkIcon, ArrowPathIcon, ExclamationTriangleIcon, ChevronDownIcon, ChevronRightIcon, DocumentTextIcon, Cog6ToothIcon } from '@heroicons/react/24/outline';
import { SearchHelmChart, UpgradeHelmRelease, GetHelmChartVersions, GetHelmReleaseValues, GetHelmReleaseAllValues, ListChartSources, SearchChartInSource } from '../../../../wailsjs/go/main/App';
import { useNotification } from '../../../context/NotificationContext';
import SearchSelect from '../../../components/shared/SearchSelect';
import Editor from '@monaco-editor/react';
import yaml from 'js-yaml';

export default function HelmUpgradeDialog({ release, onClose, onSuccess }) {
    const { addNotification } = useNotification();
    const [loading, setLoading] = useState(true);
    const [upgrading, setUpgrading] = useState(false);
    const [error, setError] = useState(null);

    // Search mode: 'auto' or 'manual'
    const [searchMode, setSearchMode] = useState('auto');
    const [searchCancelled, setSearchCancelled] = useState(false);
    const searchCancelledRef = useRef(false);
    const searchStartedRef = useRef(false);

    // All available sources (for manual mode)
    const [allSources, setAllSources] = useState([]);

    // Search progress
    const [searchProgress, setSearchProgress] = useState({ current: 0, total: 0, currentSource: '' });
    const [searchLogs, setSearchLogs] = useState([]);
    const [showLogs, setShowLogs] = useState(false);

    // Chart sources from search
    const [chartSources, setChartSources] = useState([]);
    const [selectedSourceName, setSelectedSourceName] = useState('');

    // Versions for selected source
    const [versions, setVersions] = useState([]);
    const [selectedVersion, setSelectedVersion] = useState('');
    const [loadingVersions, setLoadingVersions] = useState(false);

    // Upgrade options
    const [reuseValues, setReuseValues] = useState(true);
    const [resetValues, setResetValues] = useState(false);
    const [force, setForce] = useState(false);
    const [wait, setWait] = useState(false);

    // Values editor
    const [showValuesEditor, setShowValuesEditor] = useState(false);
    const [valuesContent, setValuesContent] = useState('');
    const [valuesFormat, setValuesFormat] = useState('yaml');
    const [showUserValuesOnly, setShowUserValuesOnly] = useState(true);
    const [loadingValues, setLoadingValues] = useState(false);
    const [valuesError, setValuesError] = useState(null);
    const editorRef = useRef(null);

    // Get selected source object
    const selectedSource = chartSources.find(s => s.repoName === selectedSourceName) || null;

    // Load values when editor is opened
    useEffect(() => {
        if (!showValuesEditor) return;

        const fetchValues = async () => {
            setLoadingValues(true);
            setValuesError(null);
            try {
                const values = showUserValuesOnly
                    ? await GetHelmReleaseValues(release.namespace, release.name)
                    : await GetHelmReleaseAllValues(release.namespace, release.name);

                const formatted = valuesFormat === 'yaml'
                    ? yaml.dump(values || {}, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: true })
                    : JSON.stringify(values || {}, null, 2);
                setValuesContent(formatted);
            } catch (err) {
                console.error('Failed to fetch values:', err);
                setValuesError(err?.message || String(err));
            } finally {
                setLoadingValues(false);
            }
        };

        fetchValues();
    }, [showValuesEditor, showUserValuesOnly, release.namespace, release.name]);

    // Convert values format
    const handleFormatChange = useCallback((newFormat) => {
        if (newFormat === valuesFormat) return;

        try {
            // Parse current content
            const parsed = valuesFormat === 'yaml'
                ? yaml.load(valuesContent)
                : JSON.parse(valuesContent);

            // Convert to new format
            const converted = newFormat === 'yaml'
                ? yaml.dump(parsed || {}, { indent: 2, lineWidth: -1, noRefs: true, sortKeys: true })
                : JSON.stringify(parsed || {}, null, 2);

            setValuesContent(converted);
            setValuesFormat(newFormat);
        } catch (err) {
            setValuesError(`Failed to convert: ${err.message}`);
        }
    }, [valuesFormat, valuesContent]);

    // Parse values from editor content
    const parseValues = useCallback(() => {
        if (!showValuesEditor || !valuesContent.trim()) {
            return {};
        }

        try {
            return valuesFormat === 'yaml'
                ? yaml.load(valuesContent) || {}
                : JSON.parse(valuesContent);
        } catch (err) {
            throw new Error(`Invalid ${valuesFormat.toUpperCase()}: ${err.message}`);
        }
    }, [showValuesEditor, valuesContent, valuesFormat]);

    // Progressive search for chart sources
    const performSearch = useCallback(async () => {
        setLoading(true);
        setError(null);
        setSearchLogs([]);
        setChartSources([]);
        setSearchCancelled(false);
        searchCancelledRef.current = false;

        try {
            // First, get all available sources
            const sources = await ListChartSources();
            setAllSources(sources || []);

            if (!sources || sources.length === 0) {
                setSearchLogs(prev => [...prev, { time: new Date(), message: 'No chart sources configured', type: 'warn' }]);
                setLoading(false);
                return;
            }

            setSearchProgress({ current: 0, total: sources.length, currentSource: '' });
            setSearchLogs(prev => [...prev, { time: new Date(), message: `Starting search for "${release.chart}" across ${sources.length} sources...`, type: 'info' }]);

            const foundSources = [];
            const chartName = release.chart;

            // Search each source progressively
            for (let i = 0; i < sources.length; i++) {
                if (searchCancelledRef.current) {
                    setSearchLogs(prev => [...prev, { time: new Date(), message: 'Search cancelled by user', type: 'warn' }]);
                    break;
                }

                const source = sources[i];
                setSearchProgress({ current: i + 1, total: sources.length, currentSource: source.name });

                try {
                    const result = await SearchChartInSource(source.name, chartName);

                    if (result.log) {
                        setSearchLogs(prev => [...prev, {
                            time: new Date(),
                            message: `${result.log} (${result.duration}ms)`,
                            type: result.found ? 'success' : 'info'
                        }]);
                    }

                    if (result.found && result.source) {
                        foundSources.push(result.source);
                        // Update chart sources progressively
                        setChartSources([...foundSources]);
                    }
                } catch (err) {
                    setSearchLogs(prev => [...prev, {
                        time: new Date(),
                        message: `[${source.name}] Error: ${err?.message || err}`,
                        type: 'error'
                    }]);
                }
            }

            // Sort found sources by priority
            foundSources.sort((a, b) => a.priority - b.priority);
            setChartSources(foundSources);

            // Auto-select the first (highest priority) source if available
            if (foundSources.length > 0) {
                setSelectedSourceName(foundSources[0].repoName);
                setVersions(foundSources[0].versions || []);
                const currentVersion = release.chartVersion;
                if (currentVersion && foundSources[0].versions?.some(v => v.version === currentVersion)) {
                    setSelectedVersion(currentVersion);
                } else if (foundSources[0].versions?.length > 0) {
                    setSelectedVersion(foundSources[0].versions[0].version);
                }
            }

            const msg = foundSources.length > 0
                ? `Search complete. Found ${foundSources.length} source(s) with chart "${chartName}".`
                : `Search complete. Chart "${chartName}" not found in any source.`;
            setSearchLogs(prev => [...prev, { time: new Date(), message: msg, type: foundSources.length > 0 ? 'success' : 'warn' }]);
        } catch (err) {
            console.error('Failed to search charts:', err);
            setError(`Failed to search: ${err?.message || err}`);
            setSearchLogs(prev => [...prev, { time: new Date(), message: `Error: ${err?.message || err}`, type: 'error' }]);
        } finally {
            setLoading(false);
            setSearchProgress({ current: 0, total: 0, currentSource: '' });
        }
    }, [release.chart, release.chartVersion]);

    // Cancel search (switches to manual mode)
    const handleCancelSearch = useCallback(() => {
        searchCancelledRef.current = true;
        setSearchCancelled(true);
        setSearchMode('manual');
    }, []);

    // Stop search (keeps current results, stays in auto mode)
    const handleStopSearch = useCallback(() => {
        searchCancelledRef.current = true;
        setSearchCancelled(true);
    }, []);

    // Switch to manual mode
    const handleSwitchToManual = useCallback(async () => {
        searchCancelledRef.current = true;
        setSearchCancelled(true);
        setSearchMode('manual');
        setLoading(false);

        // Load all sources if not already loaded
        if (allSources.length === 0) {
            try {
                const sources = await ListChartSources();
                setAllSources(sources || []);
            } catch (err) {
                console.error('Failed to load sources:', err);
            }
        }
    }, [allSources.length]);

    // Manual source selection and validation
    const handleManualSourceSelect = useCallback(async (sourceName) => {
        setSelectedSourceName(sourceName);
        setLoadingVersions(true);
        setError(null);
        setVersions([]);
        setSelectedVersion('');

        try {
            const result = await SearchChartInSource(sourceName, release.chart);
            setSearchLogs(prev => [...prev, {
                time: new Date(),
                message: result.log || `Checked ${sourceName}`,
                type: result.found ? 'success' : 'warn'
            }]);

            if (result.found && result.source) {
                setChartSources([result.source]);
                setVersions(result.source.versions || []);
                if (result.source.versions?.length > 0) {
                    const currentVersion = release.chartVersion;
                    if (currentVersion && result.source.versions.some(v => v.version === currentVersion)) {
                        setSelectedVersion(currentVersion);
                    } else {
                        setSelectedVersion(result.source.versions[0].version);
                    }
                }
            } else {
                setChartSources([]);
                setError(`Chart "${release.chart}" not found in ${sourceName}`);
            }
        } catch (err) {
            console.error('Failed to validate source:', err);
            setError(`Failed to validate: ${err?.message || err}`);
        } finally {
            setLoadingVersions(false);
        }
    }, [release.chart, release.chartVersion]);

    // Start search on mount (auto mode) - use ref to prevent double execution in StrictMode
    useEffect(() => {
        if (searchMode === 'auto' && !searchStartedRef.current) {
            searchStartedRef.current = true;
            performSearch();
        }
    }, []);

    // Load versions when source changes
    const handleSourceChange = useCallback(async (repoName) => {
        setSelectedSourceName(repoName);
        const source = chartSources.find(s => s.repoName === repoName);
        if (!source) return;

        setLoadingVersions(true);
        setError(null);

        try {
            // Use cached versions from the source if available
            if (source.versions && source.versions.length > 0) {
                setVersions(source.versions);
                // Default to current version if available
                const currentVersion = release.chartVersion;
                if (currentVersion && source.versions.some(v => v.version === currentVersion)) {
                    setSelectedVersion(currentVersion);
                } else {
                    setSelectedVersion(source.versions[0].version);
                }
            } else {
                // Otherwise fetch versions
                const fetchedVersions = await GetHelmChartVersions(source.repoName, source.chartName);
                setVersions(fetchedVersions || []);
                if (fetchedVersions && fetchedVersions.length > 0) {
                    setSelectedVersion(fetchedVersions[0].version);
                }
            }
        } catch (err) {
            console.error('Failed to load versions:', err);
            setError(`Failed to load versions: ${err?.message || err}`);
        } finally {
            setLoadingVersions(false);
        }
    }, [chartSources, release.chartVersion]);

    const handleUpgrade = useCallback(() => {
        if (!selectedSource || !selectedVersion) {
            setError('Please select a chart source and version');
            return;
        }

        // Parse custom values before closing (validation step)
        let customValues = {};
        try {
            if (showValuesEditor && valuesContent.trim()) {
                customValues = parseValues();
            }
        } catch (err) {
            setError(err?.message || String(err));
            return;
        }

        // Close dialog immediately - operation runs in background
        onClose();

        // Show in-progress notification
        addNotification({
            type: 'info',
            title: 'Upgrade started',
            message: `Upgrading "${release.name}" to ${selectedVersion}...`,
            duration: 3000
        });

        // Run upgrade asynchronously without blocking
        UpgradeHelmRelease(release.namespace, release.name, {
            repoName: selectedSource.repoName,
            repoUrl: selectedSource.repoUrl,
            chartName: selectedSource.chartName,
            version: selectedVersion,
            values: customValues,
            reuseValues: reuseValues && !showValuesEditor, // Don't reuse if custom values provided
            resetValues: resetValues,
            force: force,
            wait: wait,
            timeout: 300, // 5 minutes
            isOci: selectedSource.isOci || false,
            ociRepository: selectedSource.ociRepository || ''
        })
            .then(() => {
                addNotification({
                    type: 'success',
                    title: 'Upgrade complete',
                    message: `"${release.name}" upgraded to ${selectedVersion}`
                });
                onSuccess();
            })
            .catch((err) => {
                console.error('Failed to upgrade release:', err);
                addNotification({
                    type: 'error',
                    title: 'Upgrade failed',
                    message: err?.message || String(err)
                });
            });
    }, [release, selectedSource, selectedVersion, reuseValues, resetValues, force, wait, onClose, onSuccess, showValuesEditor, valuesContent, parseValues, addNotification]);

    const isCurrentVersion = selectedVersion === release.chartVersion &&
                            selectedSource?.repoName &&
                            selectedSource?.chartName;

    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-surface border border-border rounded-lg shadow-xl w-full max-w-2xl max-h-[90vh] overflow-hidden flex flex-col">
                {/* Header */}
                <div className="flex items-center justify-between px-6 py-4 border-b border-border shrink-0">
                    <div>
                        <h2 className="text-lg font-semibold">Upgrade Release</h2>
                        <p className="text-sm text-gray-400 mt-1">
                            {release.name} in {release.namespace}
                        </p>
                    </div>
                    <button
                        onClick={onClose}
                        className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                    >
                        <XMarkIcon className="h-5 w-5" />
                    </button>
                </div>

                {/* Content */}
                <div className="flex-1 overflow-y-auto p-6 space-y-6">
                    {/* Current Release Info */}
                    <div className="p-4 bg-background rounded-md">
                        <div className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">Current Version</div>
                        <div className="font-mono text-sm">
                            {release.chart}-{release.chartVersion}
                            {release.appVersion && <span className="text-gray-500 ml-2">(App: {release.appVersion})</span>}
                        </div>
                    </div>

                    {/* Search Mode Toggle & Progress */}
                    {loading && searchMode === 'auto' && (
                        <div className="p-4 bg-background rounded-md space-y-3">
                            <div className="flex items-center justify-between">
                                <div className="flex items-center gap-2">
                                    <ArrowPathIcon className="h-5 w-5 animate-spin text-primary" />
                                    <span className="text-sm text-gray-300">
                                        Searching: {searchProgress.currentSource || 'Initializing...'}
                                    </span>
                                </div>
                                <button
                                    onClick={handleSwitchToManual}
                                    className="flex items-center gap-1.5 px-3 py-1 text-xs bg-surface hover:bg-white/10 border border-border rounded transition-colors"
                                >
                                    <Cog6ToothIcon className="h-3.5 w-3.5" />
                                    Manual Mode
                                </button>
                            </div>
                            {searchProgress.total > 0 && (
                                <div className="space-y-1">
                                    <div className="flex justify-between text-xs text-gray-500">
                                        <span>Checking sources...</span>
                                        <span>{searchProgress.current} / {searchProgress.total}</span>
                                    </div>
                                    <div className="w-full bg-gray-700 rounded-full h-2">
                                        <div
                                            className="bg-primary h-2 rounded-full transition-all duration-300"
                                            style={{ width: `${(searchProgress.current / searchProgress.total) * 100}%` }}
                                        />
                                    </div>
                                </div>
                            )}
                            {chartSources.length > 0 && (
                                <div className="flex items-center justify-between">
                                    <span className="text-xs text-green-400">
                                        Found {chartSources.length} source(s) so far...
                                    </span>
                                    <button
                                        onClick={handleStopSearch}
                                        className="text-xs text-gray-400 hover:text-white px-2 py-0.5 bg-surface hover:bg-white/10 border border-border rounded transition-colors"
                                    >
                                        Stop
                                    </button>
                                </div>
                            )}
                        </div>
                    )}

                    {/* Manual Mode UI */}
                    {searchMode === 'manual' && !loading && (
                        <div className="p-4 bg-background rounded-md space-y-3">
                            <div className="flex items-center justify-between">
                                <span className="text-sm font-medium text-gray-300">Manual Source Selection</span>
                                <button
                                    onClick={() => { setSearchMode('auto'); searchStartedRef.current = false; performSearch(); }}
                                    className="flex items-center gap-1.5 px-3 py-1 text-xs bg-primary/20 hover:bg-primary/30 text-primary rounded transition-colors"
                                >
                                    <ArrowPathIcon className="h-3.5 w-3.5" />
                                    Auto Search
                                </button>
                            </div>
                            <div>
                                <label className="block text-xs text-gray-500 mb-1.5">Select a source to check:</label>
                                <SearchSelect
                                    options={allSources}
                                    value={selectedSourceName}
                                    onChange={handleManualSourceSelect}
                                    placeholder="Select source..."
                                    disabled={loadingVersions}
                                    getOptionValue={(source) => source.name}
                                    getOptionLabel={(source) => `${source.name} (priority: ${source.priority})`}
                                    renderOption={(source, isSelected) => (
                                        <div className="flex-1">
                                            <div className={isSelected ? 'text-primary' : ''}>
                                                {source.name}
                                                {source.isOci && (
                                                    <span className="ml-2 text-xs bg-purple-500/20 text-purple-400 px-1.5 py-0.5 rounded">OCI</span>
                                                )}
                                                {source.isAcr && (
                                                    <span className="ml-1 text-xs bg-blue-500/20 text-blue-400 px-1.5 py-0.5 rounded">ACR</span>
                                                )}
                                                <span className="text-gray-500 ml-2 text-xs">priority: {source.priority}</span>
                                            </div>
                                            <div className="text-xs text-gray-500 truncate">{source.url}</div>
                                        </div>
                                    )}
                                />
                            </div>
                            {loadingVersions && (
                                <div className="flex items-center gap-2 text-sm text-gray-400">
                                    <ArrowPathIcon className="h-4 w-4 animate-spin" />
                                    Checking source...
                                </div>
                            )}
                            {selectedSource && !loadingVersions && (
                                <div className="text-xs text-green-400 flex items-center gap-1.5">
                                    <span>✓ Chart found:</span>
                                    <span className="text-gray-400 font-mono">
                                        {selectedSource.isOci
                                            ? `oci://${selectedSource.repoUrl.replace(/^https?:\/\//, '')}/${selectedSource.ociRepository}`
                                            : selectedSource.repoUrl}
                                    </span>
                                </div>
                            )}
                        </div>
                    )}

                    {/* No sources found (after search) */}
                    {!loading && searchMode === 'auto' && chartSources.length === 0 && (
                        <div className="p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-md">
                            <div className="flex items-start gap-3">
                                <ExclamationTriangleIcon className="h-5 w-5 text-yellow-500 shrink-0 mt-0.5" />
                                <div className="flex-1">
                                    <div className="font-medium text-yellow-500">No chart sources found</div>
                                    <p className="text-sm text-gray-400 mt-1">
                                        Could not find chart "{release.chart}" in any configured chart source.
                                    </p>
                                    <button
                                        onClick={handleSwitchToManual}
                                        className="mt-2 text-xs text-primary hover:text-primary/80 underline"
                                    >
                                        Try manual source selection
                                    </button>
                                </div>
                            </div>
                        </div>
                    )}

                    {/* Chart sources found - show source dropdown only in auto mode */}
                    {!loading && chartSources.length > 0 && (
                        <>
                            {/* Chart Source Selection - only show in auto mode (manual mode has its own dropdown) */}
                            {searchMode === 'auto' && (
                                <div>
                                    <label className="block text-sm font-medium text-gray-300 mb-2">
                                        Chart Source
                                    </label>
                                    <SearchSelect
                                        options={chartSources}
                                        value={selectedSourceName}
                                        onChange={handleSourceChange}
                                        placeholder="Select chart source..."
                                        disabled={upgrading}
                                        getOptionValue={(source) => source.repoName}
                                        getOptionLabel={(source) => `${source.repoName} (priority: ${source.priority})`}
                                        renderOption={(source, isSelected) => (
                                            <div className="flex-1">
                                                <div className={isSelected ? 'text-primary' : ''}>
                                                    {source.repoName}
                                                    {source.isOci && (
                                                        <span className="ml-2 text-xs bg-purple-500/20 text-purple-400 px-1.5 py-0.5 rounded">OCI</span>
                                                    )}
                                                    <span className="text-gray-500 ml-2 text-xs">priority: {source.priority}</span>
                                                </div>
                                                <div className="text-xs text-gray-500 truncate">
                                                    {source.isOci ? source.ociRepository : source.repoUrl}
                                                </div>
                                            </div>
                                        )}
                                    />
                                    {selectedSource && (
                                        <p className="mt-1 text-xs text-gray-500 font-mono truncate" title={selectedSource.isOci ? `oci://${selectedSource.repoUrl.replace(/^https?:\/\//, '')}/${selectedSource.ociRepository}` : selectedSource.repoUrl}>
                                            {selectedSource.isOci
                                                ? `oci://${selectedSource.repoUrl.replace(/^https?:\/\//, '')}/${selectedSource.ociRepository}`
                                                : selectedSource.repoUrl}
                                        </p>
                                    )}
                                </div>
                            )}

                            {/* Version Selection */}
                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-2">
                                    Version
                                    {loadingVersions && <ArrowPathIcon className="h-4 w-4 animate-spin inline ml-2" />}
                                </label>
                                <SearchSelect
                                    options={versions}
                                    value={selectedVersion}
                                    onChange={setSelectedVersion}
                                    placeholder="Select version..."
                                    disabled={upgrading || loadingVersions || versions.length === 0}
                                    getOptionValue={(v) => v.version}
                                    getOptionLabel={(v) => {
                                        let label = v.version;
                                        if (v.appVersion) label += ` (App: ${v.appVersion})`;
                                        if (v.version === release.chartVersion) label += ' (current)';
                                        if (v.deprecated) label += ' [DEPRECATED]';
                                        return label;
                                    }}
                                    renderOption={(v, isSelected) => (
                                        <div className="flex-1 flex items-center justify-between gap-2">
                                            <div>
                                                <span className={isSelected ? 'text-primary' : ''}>
                                                    {v.version}
                                                </span>
                                                {v.version === release.chartVersion && (
                                                    <span className="ml-2 text-xs bg-primary/20 text-primary px-1.5 py-0.5 rounded">current</span>
                                                )}
                                                {v.deprecated && (
                                                    <span className="ml-2 text-xs bg-yellow-500/20 text-yellow-500 px-1.5 py-0.5 rounded">deprecated</span>
                                                )}
                                            </div>
                                            {v.appVersion && (
                                                <span className="text-xs text-gray-500">App: {v.appVersion}</span>
                                            )}
                                        </div>
                                    )}
                                />
                            </div>

                            {/* Upgrade Options */}
                            <div>
                                <div className="text-sm font-medium text-gray-300 mb-3">Options</div>
                                <div className="space-y-3">
                                    <label className={`flex items-start gap-3 ${showValuesEditor ? 'opacity-50' : ''}`}>
                                        <input
                                            type="checkbox"
                                            checked={reuseValues && !showValuesEditor}
                                            onChange={(e) => {
                                                setReuseValues(e.target.checked);
                                                if (e.target.checked) setResetValues(false);
                                            }}
                                            disabled={upgrading || showValuesEditor}
                                            className="mt-1"
                                        />
                                        <div>
                                            <div className="text-sm">Reuse Values</div>
                                            <div className="text-xs text-gray-500">
                                                Keep the current release's values and merge with any new defaults
                                                {showValuesEditor && <span className="text-yellow-500 ml-1">(disabled when editing values)</span>}
                                            </div>
                                        </div>
                                    </label>

                                    <label className="flex items-start gap-3">
                                        <input
                                            type="checkbox"
                                            checked={resetValues}
                                            onChange={(e) => {
                                                setResetValues(e.target.checked);
                                                if (e.target.checked) setReuseValues(false);
                                            }}
                                            disabled={upgrading}
                                            className="mt-1"
                                        />
                                        <div>
                                            <div className="text-sm">Reset Values</div>
                                            <div className="text-xs text-gray-500">Reset values to the chart's defaults</div>
                                        </div>
                                    </label>

                                    <label className="flex items-start gap-3">
                                        <input
                                            type="checkbox"
                                            checked={force}
                                            onChange={(e) => setForce(e.target.checked)}
                                            disabled={upgrading}
                                            className="mt-1"
                                        />
                                        <div>
                                            <div className="text-sm">Force</div>
                                            <div className="text-xs text-gray-500">Force resource updates through a replacement strategy</div>
                                        </div>
                                    </label>

                                    <label className="flex items-start gap-3">
                                        <input
                                            type="checkbox"
                                            checked={wait}
                                            onChange={(e) => setWait(e.target.checked)}
                                            disabled={upgrading}
                                            className="mt-1"
                                        />
                                        <div>
                                            <div className="text-sm">Wait</div>
                                            <div className="text-xs text-gray-500">Wait until all resources are ready before marking the release as successful</div>
                                        </div>
                                    </label>
                                </div>
                            </div>

                            {/* Values Editor */}
                            <div className="border border-border rounded-md overflow-hidden">
                                <button
                                    type="button"
                                    onClick={() => setShowValuesEditor(!showValuesEditor)}
                                    className="w-full flex items-center justify-between px-4 py-3 bg-background hover:bg-white/5 transition-colors"
                                    disabled={upgrading}
                                >
                                    <div className="flex items-center gap-2">
                                        {showValuesEditor ? (
                                            <ChevronDownIcon className="h-4 w-4 text-gray-400" />
                                        ) : (
                                            <ChevronRightIcon className="h-4 w-4 text-gray-400" />
                                        )}
                                        <span className="text-sm font-medium text-gray-300">Adjust Values</span>
                                    </div>
                                    {showValuesEditor && (
                                        <span className="text-xs text-primary">Editing</span>
                                    )}
                                </button>

                                {showValuesEditor && (
                                    <div className="border-t border-border">
                                        {/* Values Editor Toolbar */}
                                        <div className="flex items-center justify-between px-3 py-2 bg-surface border-b border-border">
                                            <label className="flex items-center gap-2 text-xs text-gray-400">
                                                <input
                                                    type="checkbox"
                                                    checked={showUserValuesOnly}
                                                    onChange={(e) => setShowUserValuesOnly(e.target.checked)}
                                                    disabled={loadingValues}
                                                />
                                                User values only
                                            </label>

                                            <div className="flex items-center gap-2">
                                                {/* Format Toggle */}
                                                <div className="flex items-center bg-surface-light rounded p-0.5">
                                                    <button
                                                        type="button"
                                                        onClick={() => handleFormatChange('yaml')}
                                                        className={`px-2 py-0.5 text-xs font-medium rounded transition-colors ${
                                                            valuesFormat === 'yaml'
                                                                ? 'bg-primary text-white'
                                                                : 'text-gray-400 hover:text-white'
                                                        }`}
                                                    >
                                                        YAML
                                                    </button>
                                                    <button
                                                        type="button"
                                                        onClick={() => handleFormatChange('json')}
                                                        className={`px-2 py-0.5 text-xs font-medium rounded transition-colors ${
                                                            valuesFormat === 'json'
                                                                ? 'bg-primary text-white'
                                                                : 'text-gray-400 hover:text-white'
                                                        }`}
                                                    >
                                                        JSON
                                                    </button>
                                                </div>
                                            </div>
                                        </div>

                                        {/* Editor */}
                                        <div className="h-48 bg-background">
                                            {loadingValues ? (
                                                <div className="flex items-center justify-center h-full text-gray-400">
                                                    <ArrowPathIcon className="h-5 w-5 animate-spin mr-2" />
                                                    Loading values...
                                                </div>
                                            ) : valuesError ? (
                                                <div className="flex items-center justify-center h-full text-red-400 text-sm px-4">
                                                    {valuesError}
                                                </div>
                                            ) : (
                                                <Editor
                                                    height="100%"
                                                    language={valuesFormat}
                                                    value={valuesContent}
                                                    onChange={(value) => setValuesContent(value || '')}
                                                    theme="vs-dark"
                                                    onMount={(editor) => { editorRef.current = editor; }}
                                                    options={{
                                                        readOnly: upgrading,
                                                        minimap: { enabled: false },
                                                        scrollBeyondLastLine: false,
                                                        fontSize: 12,
                                                        lineNumbers: 'on',
                                                        renderLineHighlight: 'line',
                                                        automaticLayout: true,
                                                        wordWrap: 'on',
                                                        folding: true,
                                                        foldingStrategy: 'indentation',
                                                        lineNumbersMinChars: 3,
                                                        glyphMargin: false,
                                                        lineDecorationsWidth: 0
                                                    }}
                                                />
                                            )}
                                        </div>
                                    </div>
                                )}
                            </div>

                            {/* Warning for same version */}
                            {isCurrentVersion && (
                                <div className="p-3 bg-yellow-500/10 border border-yellow-500/30 rounded-md text-yellow-400 text-sm">
                                    You are upgrading to the same version. This will reinstall the release.
                                </div>
                            )}
                        </>
                    )}

                    {/* Error */}
                    {error && (
                        <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-md text-red-400 text-sm">
                            {error}
                        </div>
                    )}

                    {/* Search Logs */}
                    {searchLogs.length > 0 && (
                        <div className="border border-border rounded-md overflow-hidden">
                            <button
                                type="button"
                                onClick={() => setShowLogs(!showLogs)}
                                className="w-full flex items-center justify-between px-4 py-2 bg-background hover:bg-white/5 transition-colors"
                            >
                                <div className="flex items-center gap-2">
                                    {showLogs ? (
                                        <ChevronDownIcon className="h-4 w-4 text-gray-400" />
                                    ) : (
                                        <ChevronRightIcon className="h-4 w-4 text-gray-400" />
                                    )}
                                    <DocumentTextIcon className="h-4 w-4 text-gray-400" />
                                    <span className="text-sm text-gray-300">Search Log</span>
                                    <span className="text-xs text-gray-500">({searchLogs.length} entries)</span>
                                </div>
                                <button
                                    type="button"
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        const logText = searchLogs.map(l => `[${l.time.toLocaleTimeString()}] ${l.message}`).join('\n');
                                        navigator.clipboard.writeText(logText);
                                    }}
                                    className="text-xs text-gray-500 hover:text-gray-300 px-2 py-0.5 bg-surface rounded"
                                >
                                    Copy
                                </button>
                            </button>
                            {showLogs && (
                                <div className="border-t border-border bg-background-dark max-h-40 overflow-y-auto font-mono text-xs">
                                    {searchLogs.map((log, i) => (
                                        <div
                                            key={i}
                                            className={`px-4 py-1 border-b border-border/50 ${
                                                log.type === 'error' ? 'text-red-400' :
                                                log.type === 'success' ? 'text-green-400' :
                                                log.type === 'warn' ? 'text-yellow-400' :
                                                'text-gray-400'
                                            }`}
                                        >
                                            <span className="text-gray-600">[{log.time.toLocaleTimeString()}]</span>{' '}
                                            {log.message}
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}
                </div>

                {/* Footer */}
                <div className="flex justify-end gap-3 px-6 py-4 border-t border-border shrink-0">
                    <button
                        type="button"
                        onClick={onClose}
                        disabled={upgrading}
                        className="px-4 py-2 text-sm text-gray-400 hover:text-white hover:bg-white/10 rounded-md transition-colors disabled:opacity-50"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={handleUpgrade}
                        disabled={upgrading || loading || chartSources.length === 0 || !selectedVersion}
                        className="px-4 py-2 text-sm bg-primary hover:bg-primary/80 text-white rounded-md transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                    >
                        {upgrading && (
                            <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
                        )}
                        {isCurrentVersion ? 'Reinstall' : 'Upgrade'}
                    </button>
                </div>
            </div>
        </div>
    );
}
