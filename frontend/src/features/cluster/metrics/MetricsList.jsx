import React, { useState, useEffect, useCallback } from 'react';
import { ChartBarIcon, CheckCircleIcon, XCircleIcon, ArrowPathIcon, ServerStackIcon, ArrowTopRightOnSquareIcon, CpuChipIcon } from '@heroicons/react/24/outline';
import { GetNodeMetrics, DetectPrometheus, ListPrometheusInstalls, TestPrometheusEndpoint, SavePrometheusConfig, ClearPrometheusConfig, AddPortForwardConfig, StartPortForward, GetRandomAvailablePort, GetPortForwardConfigs, GetActivePortForwards } from '../../../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../../../wailsjs/runtime/runtime';
import { useK8s } from '../../../context/K8sContext';
import { useConfig } from '../../../context/ConfigContext';
import SourceSelect, { sourceOptions } from '../../../components/shared/SourceSelect';

export default function MetricsList({ isVisible }) {
    const { currentContext } = useK8s();
    const { getConfig, setConfig } = useConfig();

    // Direct K8s Metrics API check (bypasses dual-source logic so we test metrics-server specifically)
    const [k8sMetricsAvailable, setK8sMetricsAvailable] = useState(null);
    const [k8sMetricsLoading, setK8sMetricsLoading] = useState(false);

    const checkK8sMetrics = useCallback(async () => {
        if (!currentContext || !isVisible) return;
        setK8sMetricsLoading(true);
        try {
            const result = await GetNodeMetrics();
            setK8sMetricsAvailable(result?.available ?? false);
        } catch {
            setK8sMetricsAvailable(false);
        } finally {
            setK8sMetricsLoading(false);
        }
    }, [currentContext, isVisible]);

    useEffect(() => {
        if (isVisible) {
            checkK8sMetrics();
        }
    }, [checkK8sMetrics, isVisible]);

    // Reset on context change
    useEffect(() => {
        setK8sMetricsAvailable(null);
    }, [currentContext]);

    // Metrics source preference
    const preferredSource = getConfig('metrics.preferredSource') ?? 'auto';
    const handleSourceChange = (newValue) => {
        setConfig('metrics.preferredSource', newValue);
    };

    const [prometheusInfo, setPrometheusInfo] = useState(null);
    const [allInstalls, setAllInstalls] = useState([]);
    const [detecting, setDetecting] = useState(true);
    const [testing, setTesting] = useState(false);
    const [testResult, setTestResult] = useState(null);
    const [openingUI, setOpeningUI] = useState(null); // Track which install is being opened

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
        checkK8sMetrics(); // Also re-check K8s metrics API
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

    // Open Prometheus UI by port-forwarding and opening browser
    // Reuses existing port forward if available
    const openPrometheusUI = async (namespace, service, port, installKey = 'active') => {
        setOpeningUI(installKey);
        try {
            // Check for existing port forward config for this service
            const configs = await GetPortForwardConfigs(currentContext);
            const existingConfig = configs?.find(c =>
                c.namespace === namespace &&
                c.resourceName === service &&
                c.resourceType === 'service' &&
                c.remotePort === port
            );

            let localPort;
            let configId;

            if (existingConfig) {
                // Found existing config - check if it's active
                const activeForwards = await GetActivePortForwards();
                const isActive = activeForwards?.some(a => a.config?.id === existingConfig.id);

                localPort = existingConfig.localPort;
                configId = existingConfig.id;

                if (!isActive) {
                    // Start the existing port forward
                    await StartPortForward(configId);
                    await new Promise(resolve => setTimeout(resolve, 500));
                }
            } else {
                // No existing config - create new one
                localPort = await GetRandomAvailablePort();

                const config = {
                    context: currentContext,
                    namespace: namespace,
                    resourceType: 'service',
                    resourceName: service,
                    localPort: localPort,
                    remotePort: port,
                    label: `Prometheus UI (${service})`,
                    favorite: false,
                    https: false
                };

                const result = await AddPortForwardConfig(config);
                configId = result?.id;

                if (configId) {
                    await StartPortForward(configId);
                    await new Promise(resolve => setTimeout(resolve, 500));
                }
            }

            // Open in browser
            BrowserOpenURL(`http://localhost:${localPort}`);
            setTestResult({ success: true, message: `Opened Prometheus UI on localhost:${localPort}` });
        } catch (err) {
            console.error('Failed to open Prometheus UI:', err);
            setTestResult({ success: false, message: `Failed to open UI: ${err.toString()}` });
        } finally {
            setOpeningUI(null);
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
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0 titlebar-drag">
                <div className="flex items-center gap-3">
                    <ChartBarIcon className="h-6 w-6 text-primary" />
                    <h1 className="text-lg font-semibold text-text">Metrics</h1>
                </div>
                <div className="flex items-center gap-3">
                    <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-500">Source:</span>
                        <SourceSelect
                            value={preferredSource}
                            onChange={handleSourceChange}
                            options={sourceOptions}
                        />
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
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto p-6">
                <div className="max-w-2xl mx-auto space-y-6">
                    {/* Kubernetes Metrics API (metrics-server) Status */}
                    <div className="bg-surface border border-border rounded-lg p-6">
                        <h2 className="text-lg font-medium text-text mb-4 flex items-center gap-2">
                            <CpuChipIcon className="h-5 w-5" />
                            Kubernetes Metrics API
                        </h2>
                        {k8sMetricsLoading || k8sMetricsAvailable === null ? (
                            <div className="flex items-center gap-3 text-gray-400">
                                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                                Checking metrics-server availability...
                            </div>
                        ) : k8sMetricsAvailable ? (
                            <div className="space-y-3">
                                <div className="flex items-center gap-2 text-green-400">
                                    <CheckCircleIcon className="h-5 w-5" />
                                    <span className="font-medium">Available</span>
                                    <span className="text-xs bg-green-500/20 text-green-400 px-2 py-0.5 rounded">
                                        metrics-server
                                    </span>
                                </div>
                                <p className="text-sm text-gray-400">
                                    The Kubernetes Metrics API is available. Real-time CPU and memory usage data
                                    is displayed in the Nodes and Pods lists.
                                </p>
                            </div>
                        ) : (
                            <div className="space-y-3">
                                <div className="flex items-center gap-2 text-yellow-400">
                                    <XCircleIcon className="h-5 w-5" />
                                    <span className="font-medium">Not Available</span>
                                </div>
                                <p className="text-sm text-gray-400 mb-3">
                                    The Kubernetes Metrics Server is not installed or not responding.
                                    Install it to see real-time CPU and memory metrics.
                                </p>
                                <div className="bg-background rounded p-3 text-sm font-mono text-gray-300 overflow-x-auto">
                                    kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
                                </div>
                                <p className="text-xs text-gray-500">
                                    Note: For local clusters (minikube, kind, Docker Desktop), the metrics server may need
                                    additional configuration like <code className="bg-background px-1 rounded">--kubelet-insecure-tls</code>.
                                </p>
                            </div>
                        )}
                    </div>

                    {/* Prometheus - Active Connection */}
                    <div className="bg-surface border border-border rounded-lg p-6">
                        <h2 className="text-lg font-medium text-text mb-4">Prometheus Connection</h2>

                        {detecting ? (
                            <div className="flex items-center gap-3 text-gray-400">
                                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                                Detecting Prometheus installations...
                            </div>
                        ) : prometheusInfo?.available ? (
                            <div className="space-y-3">
                                <div className="flex items-center justify-between">
                                    <div className="flex items-center gap-2 text-green-400">
                                        <CheckCircleIcon className="h-5 w-5" />
                                        <span className="font-medium">Connected</span>
                                        {prometheusInfo.detectionMethod && (
                                            <span className="text-xs bg-green-500/20 text-green-400 px-2 py-0.5 rounded">
                                                {prometheusInfo.detectionMethod === 'crd' ? 'Operator' : 'Service'}
                                            </span>
                                        )}
                                    </div>
                                    <button
                                        onClick={() => openPrometheusUI(prometheusInfo.namespace, prometheusInfo.service, prometheusInfo.port, 'active')}
                                        disabled={openingUI === 'active'}
                                        className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-primary hover:bg-primary/90 text-white rounded transition-colors disabled:opacity-50"
                                    >
                                        <ArrowTopRightOnSquareIcon className="h-4 w-4" />
                                        {openingUI === 'active' ? 'Opening...' : 'Open UI'}
                                    </button>
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
                                {allInstalls.map((install, idx) => {
                                    const installKey = `${install.namespace}-${install.service}-${idx}`;
                                    const isSelected = customEndpoint.namespace === install.namespace &&
                                        customEndpoint.service === install.service;

                                    return (
                                        <div
                                            key={installKey}
                                            className={`flex items-center justify-between p-3 rounded border transition-colors ${
                                                isSelected
                                                    ? 'border-primary bg-primary/10'
                                                    : 'border-border hover:border-gray-500 hover:bg-white/5'
                                            }`}
                                        >
                                            <button
                                                onClick={() => selectInstall(install)}
                                                className="flex items-center gap-3 flex-1 text-left"
                                            >
                                                <div className="h-2 w-2 rounded-full bg-blue-400" />
                                                <div>
                                                    <div className="text-sm text-text">
                                                        {install.namespace}/{install.service}:{install.port}
                                                    </div>
                                                    <div className="text-xs text-gray-500">
                                                        {install.name !== install.service && `CR: ${install.name} • `}
                                                        {install.type === 'operator' ? 'Prometheus Operator' : 'Standalone'}
                                                    </div>
                                                </div>
                                            </button>
                                            <div className="flex items-center gap-2">
                                                <button
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        openPrometheusUI(install.namespace, install.service, install.port, installKey);
                                                    }}
                                                    disabled={openingUI === installKey}
                                                    className="flex items-center gap-1 px-2 py-1 text-xs bg-primary/20 hover:bg-primary/30 text-primary rounded transition-colors disabled:opacity-50"
                                                    title="Open Prometheus UI in browser"
                                                >
                                                    <ArrowTopRightOnSquareIcon className="h-3.5 w-3.5" />
                                                    {openingUI === installKey ? 'Opening...' : 'Open UI'}
                                                </button>
                                            </div>
                                        </div>
                                    );
                                })}
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
                                <label className="block text-gray-400 mb-1">Namespace</label>
                                <input
                                    type="text"
                                    value={customEndpoint.namespace}
                                    onChange={(e) => setCustomEndpoint(prev => ({ ...prev, namespace: e.target.value }))}
                                    placeholder="monitoring"
                                    className="w-full"
                                />
                            </div>
                            <div>
                                <label className="block text-gray-400 mb-1">Service</label>
                                <input
                                    type="text"
                                    value={customEndpoint.service}
                                    onChange={(e) => setCustomEndpoint(prev => ({ ...prev, service: e.target.value }))}
                                    placeholder="prometheus"
                                    className="w-full"
                                />
                            </div>
                            <div>
                                <label className="block text-gray-400 mb-1">Port</label>
                                <input
                                    type="number"
                                    value={customEndpoint.port}
                                    onChange={(e) => setCustomEndpoint(prev => ({ ...prev, port: parseInt(e.target.value) || 9090 }))}
                                    placeholder="9090"
                                    className="w-full"
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
