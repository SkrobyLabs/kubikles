import React from 'react';
import { useUI } from '~/context';

// Volume type badge
const VolumeTypeBadge = ({ type }) => {
    const colors = {
        emptyDir: 'bg-gray-500/10 text-gray-400 border-gray-500/30',
        secret: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30',
        configMap: 'bg-blue-500/10 text-blue-400 border-blue-500/30',
        persistentVolumeClaim: 'bg-green-500/10 text-green-400 border-green-500/30',
        projected: 'bg-purple-500/10 text-purple-400 border-purple-500/30',
        hostPath: 'bg-red-500/10 text-red-400 border-red-500/30',
        downwardAPI: 'bg-cyan-500/10 text-cyan-400 border-cyan-500/30',
        nfs: 'bg-orange-500/10 text-orange-400 border-orange-500/30',
    };

    return (
        <span className={`px-2 py-0.5 text-xs rounded border ${colors[type] || 'bg-gray-500/10 text-gray-400 border-gray-500/30'}`}>
            {type}
        </span>
    );
};

// Get volume type and details from volume spec
const getVolumeInfo = (volume) => {
    if (volume.emptyDir !== undefined) {
        return {
            type: 'emptyDir',
            details: volume.emptyDir.medium ? `medium: ${volume.emptyDir.medium}` : 'memory-backed' in (volume.emptyDir || {}) ? '' : null,
            sizeLimit: volume.emptyDir.sizeLimit
        };
    }
    if (volume.secret) {
        return {
            type: 'secret',
            secretName: volume.secret.secretName,
            defaultMode: volume.secret.defaultMode,
            items: volume.secret.items
        };
    }
    if (volume.configMap) {
        return {
            type: 'configMap',
            configMapName: volume.configMap.name,
            defaultMode: volume.configMap.defaultMode,
            items: volume.configMap.items
        };
    }
    if (volume.persistentVolumeClaim) {
        return {
            type: 'persistentVolumeClaim',
            claimName: volume.persistentVolumeClaim.claimName,
            readOnly: volume.persistentVolumeClaim.readOnly
        };
    }
    if (volume.projected) {
        return {
            type: 'projected',
            defaultMode: volume.projected.defaultMode,
            sources: volume.projected.sources
        };
    }
    if (volume.hostPath) {
        return {
            type: 'hostPath',
            path: volume.hostPath.path,
            hostPathType: volume.hostPath.type
        };
    }
    if (volume.downwardAPI) {
        return {
            type: 'downwardAPI',
            items: volume.downwardAPI.items
        };
    }
    if (volume.nfs) {
        return {
            type: 'nfs',
            server: volume.nfs.server,
            path: volume.nfs.path,
            readOnly: volume.nfs.readOnly
        };
    }
    // Generic fallback for other types
    const type = Object.keys(volume).find(k => k !== 'name');
    return { type: type || 'unknown', raw: volume[type] };
};

// Render projected sources
const ProjectedSources = ({ sources }) => {
    if (!sources || sources.length === 0) return null;

    return (
        <div className="mt-2 ml-4 space-y-1">
            {sources.map((source, idx) => {
                if (source.serviceAccountToken) {
                    return (
                        <div key={idx} className="text-xs text-gray-500">
                            <span className="text-gray-400">serviceAccountToken:</span> path={source.serviceAccountToken.path}
                            {source.serviceAccountToken.expirationSeconds && `, expires=${source.serviceAccountToken.expirationSeconds}s`}
                        </div>
                    );
                }
                if (source.configMap) {
                    return (
                        <div key={idx} className="text-xs text-gray-500">
                            <span className="text-gray-400">configMap:</span> {source.configMap.name}
                            {source.configMap.items && ` (${source.configMap.items.map(i => i.key).join(', ')})`}
                        </div>
                    );
                }
                if (source.secret) {
                    return (
                        <div key={idx} className="text-xs text-gray-500">
                            <span className="text-gray-400">secret:</span> {source.secret.name}
                            {source.secret.items && ` (${source.secret.items.map(i => i.key).join(', ')})`}
                        </div>
                    );
                }
                if (source.downwardAPI) {
                    return (
                        <div key={idx} className="text-xs text-gray-500">
                            <span className="text-gray-400">downwardAPI:</span> {source.downwardAPI.items?.map(i => i.path).join(', ')}
                        </div>
                    );
                }
                return null;
            })}
        </div>
    );
};

// Volume card component
const VolumeCard = ({ volume, namespace, onNavigate }) => {
    const info = getVolumeInfo(volume);

    const handlePVCClick = (claimName) => {
        onNavigate('pvcs', `name:"${claimName}" namespace:"${namespace}"`);
    };

    const handleSecretClick = (secretName) => {
        onNavigate('secrets', `name:"${secretName}" namespace:"${namespace}"`);
    };

    const handleConfigMapClick = (configMapName) => {
        onNavigate('configmaps', `name:"${configMapName}" namespace:"${namespace}"`);
    };

    return (
        <div className="bg-surface rounded-lg border border-border p-4">
            <div className="flex items-center gap-2 mb-2">
                <div className="font-medium text-gray-200">{volume.name}</div>
                <VolumeTypeBadge type={info.type} />
            </div>

            <div className="text-sm text-gray-400 space-y-1">
                {info.type === 'persistentVolumeClaim' && (
                    <div>
                        <span className="text-gray-500">Claim: </span>
                        <button
                            onClick={() => handlePVCClick(info.claimName)}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors"
                        >
                            {info.claimName}
                        </button>
                        {info.readOnly && <span className="ml-2 text-xs text-gray-500">(readOnly)</span>}
                    </div>
                )}

                {info.type === 'secret' && (
                    <div>
                        <span className="text-gray-500">Secret: </span>
                        <button
                            onClick={() => handleSecretClick(info.secretName)}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors"
                        >
                            {info.secretName}
                        </button>
                        {info.defaultMode && <span className="ml-2 text-xs text-gray-500">(mode: {info.defaultMode.toString(8)})</span>}
                    </div>
                )}

                {info.type === 'configMap' && (
                    <div>
                        <span className="text-gray-500">ConfigMap: </span>
                        <button
                            onClick={() => handleConfigMapClick(info.configMapName)}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors"
                        >
                            {info.configMapName}
                        </button>
                        {info.defaultMode && <span className="ml-2 text-xs text-gray-500">(mode: {info.defaultMode.toString(8)})</span>}
                    </div>
                )}

                {info.type === 'emptyDir' && (
                    <div className="text-gray-500">
                        {info.sizeLimit ? `Size limit: ${info.sizeLimit}` : 'No size limit'}
                        {info.details && ` • ${info.details}`}
                    </div>
                )}

                {info.type === 'hostPath' && (
                    <div>
                        <span className="text-gray-500">Path: </span>
                        <code className="text-xs bg-black/20 px-1.5 py-0.5 rounded font-mono">{info.path}</code>
                        {info.hostPathType && <span className="ml-2 text-xs text-gray-500">(type: {info.hostPathType})</span>}
                    </div>
                )}

                {info.type === 'nfs' && (
                    <>
                        <div>
                            <span className="text-gray-500">Server: </span>
                            <code className="text-xs bg-black/20 px-1.5 py-0.5 rounded font-mono">{info.server}</code>
                        </div>
                        <div>
                            <span className="text-gray-500">Path: </span>
                            <code className="text-xs bg-black/20 px-1.5 py-0.5 rounded font-mono">{info.path}</code>
                            {info.readOnly && <span className="ml-2 text-xs text-gray-500">(readOnly)</span>}
                        </div>
                    </>
                )}

                {info.type === 'projected' && (
                    <>
                        {info.defaultMode && (
                            <div className="text-gray-500">Default mode: {info.defaultMode.toString(8)}</div>
                        )}
                        <ProjectedSources sources={info.sources} />
                    </>
                )}

                {info.type === 'downwardAPI' && info.items && (
                    <div className="text-gray-500">
                        Items: {info.items.map(i => i.path).join(', ')}
                    </div>
                )}

                {info.raw && !['emptyDir', 'secret', 'configMap', 'persistentVolumeClaim', 'projected', 'hostPath', 'nfs', 'downwardAPI'].includes(info.type) && (
                    <pre className="text-xs bg-black/20 px-2 py-1 rounded overflow-auto">
                        {JSON.stringify(info.raw, null, 2)}
                    </pre>
                )}
            </div>
        </div>
    );
};

export default function PodVolumesTab({ pod }) {
    const { navigateWithSearch } = useUI();
    const volumes = pod.spec?.volumes || [];
    const namespace = pod.metadata?.namespace;

    if (volumes.length === 0) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500">
                No volumes defined for this pod
            </div>
        );
    }

    return (
        <div className="flex flex-col h-full overflow-auto p-4">
            <div className="grid gap-3">
                {volumes.map((volume, idx) => (
                    <VolumeCard
                        key={volume.name || idx}
                        volume={volume}
                        namespace={namespace}
                        onNavigate={navigateWithSearch}
                    />
                ))}
            </div>
        </div>
    );
}
