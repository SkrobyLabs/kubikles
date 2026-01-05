import React, { useState, useEffect } from 'react';
import { ChartBarIcon, CheckCircleIcon, XCircleIcon, ArrowPathIcon, ServerStackIcon } from '@heroicons/react/24/outline';
import { DetectPrometheus, ListPrometheusInstalls, TestPrometheusEndpoint, SavePrometheusConfig, ClearPrometheusConfig } from '../../../../wailsjs/go/main/App';

export default function MetricsList({ isVisible }) {
    const [prometheusInfo, setPrometheusInfo] = useState(null);
    const [allInstalls, setAllInstalls] = useState([]);
    const [detecting, setDetecting] = useState(true);
    const [testing, setTesting] = useState(false);
    const [testResult, setTestResult] = useState(null);

    // Custom endpoint form
    const [customEndpoint, setCustomEndpoint] = useState({
        namespace: '',
        service: '',
        port: 9090,
    });

    // Detect Prometheus on mount
    useEffect(() => {
        if (!isVisible) return;
        detectPrometheus();
    }, [isVisible]);

    const detectPrometheus = async () => {
        setDetecting(true);
        setTestResult(null);
        try {
            // Fetch both detected info and all installations
            const [info, installs] = await Promise.all([
                DetectPrometheus(),
                ListPrometheusInstalls().catch(() => [])
            ]);

            setPrometheusInfo(info);
            setAllInstalls(installs || []);

            if (info?.available) {
                setCustomEndpoint({
                    namespace: info.namespace,
                    service: info.service,
                    port: info.port,
                });
            }
        } catch (err) {
            console.error('Failed to detect Prometheus:', err);
            setPrometheusInfo({ available: false });
            setAllInstalls([]);
        } finally {
            setDetecting(false);
        }
    };

    const testEndpoint = async (autoSave = false) => {
        setTesting(true);
        setTestResult(null);
        try {
            await TestPrometheusEndpoint(
                customEndpoint.namespace,
                customEndpoint.service,
                customEndpoint.port
            );

            // Auto-save on successful test
            if (autoSave) {
                await SavePrometheusConfig(
                    customEndpoint.namespace,
                    customEndpoint.service,
                    customEndpoint.port
                );
                setTestResult({ success: true, message: 'Connection successful! Saved as default.' });
                // Update the displayed info
                setPrometheusInfo({
                    available: true,
                    namespace: customEndpoint.namespace,
                    service: customEndpoint.service,
                    port: customEndpoint.port,
                    detectionMethod: 'manual'
                });
            } else {
                setTestResult({ success: true, message: 'Connection successful!' });
            }
        } catch (err) {
            setTestResult({ success: false, message: err.toString() });
        } finally {
            setTesting(false);
        }
    };

    const saveCurrentConfig = async () => {
        if (!customEndpoint.namespace || !customEndpoint.service) return;
        try {
            await SavePrometheusConfig(
                customEndpoint.namespace,
                customEndpoint.service,
                customEndpoint.port
            );
            setTestResult({ success: true, message: 'Configuration saved!' });
            setPrometheusInfo({
                available: true,
                namespace: customEndpoint.namespace,
                service: customEndpoint.service,
                port: customEndpoint.port,
                detectionMethod: 'manual'
            });
        } catch (err) {
            setTestResult({ success: false, message: 'Failed to save: ' + err.toString() });
        }
    };

    const clearSavedConfig = async () => {
        try {
            await ClearPrometheusConfig();
            setPrometheusInfo({ available: false });
            setTestResult({ success: true, message: 'Configuration cleared.' });
        } catch (err) {
            setTestResult({ success: false, message: 'Failed to clear: ' + err.toString() });
        }
    };

    const selectInstall = async (install) => {
        setCustomEndpoint({
            namespace: install.namespace,
            service: install.service,
            port: install.port,
        });
        setTestResult(null);

        // Auto-test and save on successful connection
        setTesting(true);
        try {
            await TestPrometheusEndpoint(
                install.namespace,
                install.service,
                install.port
            );
            // Auto-save on success
            await SavePrometheusConfig(
                install.namespace,
                install.service,
                install.port
            );
            setTestResult({ success: true, message: 'Connected and saved as default!' });
            setPrometheusInfo({
                available: true,
                namespace: install.namespace,
                service: install.service,
                port: install.port,
                detectionMethod: install.type === 'operator' ? 'crd' : 'service'
            });
        } catch (err) {
            setTestResult({ success: false, message: err.toString() });
        } finally {
            setTesting(false);
        }
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header */}
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0">
                <div className="flex items-center gap-3">
                    <ChartBarIcon className="h-6 w-6 text-primary" />
                    <h1 className="text-lg font-semibold text-text">Metrics</h1>
                </div>
                <button
                    onClick={detectPrometheus}
                    disabled={detecting}
                    className="flex items-center gap-2 px-3 py-1.5 text-sm text-gray-300 hover:text-white hover:bg-white/10 rounded transition-colors disabled:opacity-50"
                >
                    <ArrowPathIcon className={`h-4 w-4 ${detecting ? 'animate-spin' : ''}`} />
                    Re-detect
                </button>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto p-6">
                <div className="max-w-2xl mx-auto space-y-6">
                    {/* Active Connection */}
                    <div className="bg-surface border border-border rounded-lg p-6">
                        <h2 className="text-lg font-medium text-text mb-4">Active Connection</h2>

                        {detecting ? (
                            <div className="flex items-center gap-3 text-gray-400">
                                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                                Detecting Prometheus installations...
                            </div>
                        ) : prometheusInfo?.available ? (
                            <div className="space-y-3">
                                <div className="flex items-center gap-2 text-green-400">
                                    <CheckCircleIcon className="h-5 w-5" />
                                    <span className="font-medium">Connected</span>
                                    {prometheusInfo.detectionMethod && (
                                        <span className="text-xs bg-green-500/20 text-green-400 px-2 py-0.5 rounded">
                                            {prometheusInfo.detectionMethod === 'crd' ? 'Operator' : 'Service'}
                                        </span>
                                    )}
                                </div>
                                <div className="grid grid-cols-3 gap-4 text-sm">
                                    <div>
                                        <span className="text-gray-400">Namespace:</span>
                                        <span className="ml-2 text-text">{prometheusInfo.namespace}</span>
                                    </div>
                                    <div>
                                        <span className="text-gray-400">Service:</span>
                                        <span className="ml-2 text-text">{prometheusInfo.service}</span>
                                    </div>
                                    <div>
                                        <span className="text-gray-400">Port:</span>
                                        <span className="ml-2 text-text">{prometheusInfo.port}</span>
                                    </div>
                                </div>
                                {prometheusInfo.crdName && (
                                    <div className="text-sm text-gray-400">
                                        CR Name: <span className="text-text">{prometheusInfo.crdName}</span>
                                    </div>
                                )}
                            </div>
                        ) : (
                            <div className="flex items-center gap-2 text-yellow-400">
                                <XCircleIcon className="h-5 w-5" />
                                <span>No Prometheus installation detected</span>
                            </div>
                        )}
                    </div>

                    {/* Discovered Installations */}
                    {!detecting && allInstalls.length > 0 && (
                        <div className="bg-surface border border-border rounded-lg p-6">
                            <h2 className="text-lg font-medium text-text mb-4">
                                <div className="flex items-center gap-2">
                                    <ServerStackIcon className="h-5 w-5" />
                                    Discovered Installations ({allInstalls.length})
                                </div>
                            </h2>
                            <p className="text-sm text-gray-400 mb-4">
                                Found {allInstalls.length} Prometheus installation{allInstalls.length > 1 ? 's' : ''} in your cluster.
                                Click to select one for manual testing.
                            </p>
                            <div className="space-y-2">
                                {allInstalls.map((install, idx) => (
                                    <button
                                        key={`${install.namespace}-${install.name}-${idx}`}
                                        onClick={() => selectInstall(install)}
                                        className={`w-full flex items-center justify-between p-3 rounded border transition-colors ${
                                            customEndpoint.namespace === install.namespace &&
                                            customEndpoint.service === install.service
                                                ? 'border-primary bg-primary/10'
                                                : 'border-border hover:border-gray-500 hover:bg-white/5'
                                        }`}
                                    >
                                        <div className="flex items-center gap-3">
                                            <div className="h-2 w-2 rounded-full bg-blue-400" />
                                            <div className="text-left">
                                                <div className="text-sm text-text">
                                                    {install.namespace}/{install.service}:{install.port}
                                                </div>
                                                <div className="text-xs text-gray-500">
                                                    {install.name !== install.service && `CR: ${install.name} • `}
                                                    {install.type === 'operator' ? 'Prometheus Operator' : 'Standalone'}
                                                </div>
                                            </div>
                                        </div>
                                        <div className="flex items-center gap-2">
                                            <span className="text-xs text-gray-400">Click to test</span>
                                        </div>
                                    </button>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* Custom Endpoint Section */}
                    <div className="bg-surface border border-border rounded-lg p-6">
                        <h2 className="text-lg font-medium text-text mb-4">Custom Endpoint</h2>
                        <p className="text-sm text-gray-400 mb-4">
                            Configure a custom Prometheus endpoint or test a discovered installation:
                        </p>

                        <div className="grid grid-cols-3 gap-4 mb-4">
                            <div>
                                <label className="block text-sm text-gray-400 mb-1">Namespace</label>
                                <input
                                    type="text"
                                    value={customEndpoint.namespace}
                                    onChange={(e) => setCustomEndpoint(prev => ({ ...prev, namespace: e.target.value }))}
                                    placeholder="monitoring"
                                    className="w-full bg-background border border-border rounded px-3 py-2 text-sm text-text focus:outline-none focus:border-primary"
                                />
                            </div>
                            <div>
                                <label className="block text-sm text-gray-400 mb-1">Service</label>
                                <input
                                    type="text"
                                    value={customEndpoint.service}
                                    onChange={(e) => setCustomEndpoint(prev => ({ ...prev, service: e.target.value }))}
                                    placeholder="prometheus"
                                    className="w-full bg-background border border-border rounded px-3 py-2 text-sm text-text focus:outline-none focus:border-primary"
                                />
                            </div>
                            <div>
                                <label className="block text-sm text-gray-400 mb-1">Port</label>
                                <input
                                    type="number"
                                    value={customEndpoint.port}
                                    onChange={(e) => setCustomEndpoint(prev => ({ ...prev, port: parseInt(e.target.value) || 9090 }))}
                                    placeholder="9090"
                                    className="w-full bg-background border border-border rounded px-3 py-2 text-sm text-text focus:outline-none focus:border-primary"
                                />
                            </div>
                        </div>

                        <div className="flex items-center gap-2 flex-wrap">
                            <button
                                onClick={() => testEndpoint(false)}
                                disabled={testing || !customEndpoint.namespace || !customEndpoint.service}
                                className="px-4 py-2 text-sm bg-gray-600 hover:bg-gray-500 text-white rounded transition-colors disabled:opacity-50"
                            >
                                {testing ? 'Testing...' : 'Test'}
                            </button>
                            <button
                                onClick={() => testEndpoint(true)}
                                disabled={testing || !customEndpoint.namespace || !customEndpoint.service}
                                className="px-4 py-2 text-sm bg-primary hover:bg-primary/90 text-white rounded transition-colors disabled:opacity-50"
                            >
                                Test & Save
                            </button>
                            {prometheusInfo?.available && (
                                <button
                                    onClick={clearSavedConfig}
                                    className="px-4 py-2 text-sm bg-red-600/20 hover:bg-red-600/30 text-red-400 rounded transition-colors"
                                >
                                    Clear Saved
                                </button>
                            )}
                        </div>

                        {testResult && (
                            <div className={`flex items-center gap-2 text-sm mt-3 ${testResult.success ? 'text-green-400' : 'text-red-400'}`}>
                                {testResult.success ? (
                                    <CheckCircleIcon className="h-4 w-4" />
                                ) : (
                                    <XCircleIcon className="h-4 w-4" />
                                )}
                                {testResult.message}
                            </div>
                        )}
                    </div>

                    {/* Info Section */}
                    <div className="bg-surface border border-border rounded-lg p-6">
                        <h2 className="text-lg font-medium text-text mb-4">About Metrics</h2>
                        <div className="text-sm text-gray-400 space-y-3">
                            <p>
                                Kubikles integrates with Prometheus to display historical CPU and memory metrics
                                for your pods. This feature requires a Prometheus installation in your cluster.
                            </p>
                            <p>
                                <strong className="text-gray-300">Detection methods:</strong>
                            </p>
                            <ul className="list-disc list-inside ml-2 space-y-1">
                                <li><strong className="text-gray-300">CRD-based:</strong> Detects Prometheus Operator installations by querying <code className="text-xs bg-background px-1 py-0.5 rounded">prometheuses.monitoring.coreos.com</code> CRs</li>
                                <li><strong className="text-gray-300">Service-based:</strong> Falls back to searching for services with known patterns/labels</li>
                            </ul>
                            <p>
                                <strong className="text-gray-300">Supported installations:</strong>
                            </p>
                            <ul className="list-disc list-inside ml-2 space-y-1">
                                <li>kube-prometheus-stack</li>
                                <li>prometheus-operator</li>
                                <li>Standalone Prometheus</li>
                            </ul>
                            <p>
                                Once connected, you can view historical metrics in the <strong className="text-gray-300">Metrics</strong> tab
                                of any pod's details view.
                            </p>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}
