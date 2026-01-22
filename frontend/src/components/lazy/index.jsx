/**
 * Lazy-loaded heavy components
 *
 * These components use React.lazy() to defer loading of large dependencies
 * (Monaco ~1MB, React Flow ~400KB, xterm ~280KB) until they're actually needed.
 *
 * Usage:
 *   import { LazyYamlEditor, LazyDependencyGraph, LazyTerminal } from '../lazy';
 */
import React, { Suspense, lazy } from 'react';

// Loading spinner component
const LoadingSpinner = ({ text = 'Loading...' }) => (
    <div className="flex items-center justify-center h-full w-full bg-background">
        <div className="flex flex-col items-center gap-3">
            <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
            <span className="text-sm text-gray-400">{text}</span>
        </div>
    </div>
);

// Lazy load the heavy components
const YamlEditorImpl = lazy(() => import('../shared/YamlEditor'));
const DependencyGraphImpl = lazy(() => import('../shared/DependencyGraph'));
const TerminalImpl = lazy(() => import('../shared/Terminal'));

// Monaco editor direct (for ConfigEditor, SecretEditor, etc.)
const MonacoEditorImpl = lazy(() =>
    import('@monaco-editor/react').then(mod => ({ default: mod.default }))
);

/**
 * Lazy-loaded YAML Editor with Suspense boundary
 */
export const LazyYamlEditor = (props) => (
    <Suspense fallback={<LoadingSpinner text="Loading editor..." />}>
        <YamlEditorImpl {...props} />
    </Suspense>
);

/**
 * Lazy-loaded Dependency Graph with Suspense boundary
 */
export const LazyDependencyGraph = (props) => (
    <Suspense fallback={<LoadingSpinner text="Loading graph..." />}>
        <DependencyGraphImpl {...props} />
    </Suspense>
);

/**
 * Lazy-loaded Terminal with Suspense boundary
 */
export const LazyTerminal = (props) => (
    <Suspense fallback={<LoadingSpinner text="Connecting..." />}>
        <TerminalImpl {...props} />
    </Suspense>
);

/**
 * Lazy-loaded Monaco Editor with Suspense boundary
 * Use this for direct Monaco usage (ConfigEditor, SecretEditor, etc.)
 */
export const LazyMonacoEditor = (props) => (
    <Suspense fallback={<LoadingSpinner text="Loading editor..." />}>
        <MonacoEditorImpl {...props} />
    </Suspense>
);

// Also export the loading spinner for custom use
export { LoadingSpinner };
