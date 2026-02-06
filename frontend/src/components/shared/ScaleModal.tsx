import React, { useState, useEffect } from 'react';
import { MinusIcon, PlusIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { ListPDBs } from 'wailsjs/go/main/App';
import { useForm, scaleSchema } from '~/lib/validation';

interface ScaleModalProps {
    resourceType: string;
    resourceName: string;
    namespace: string;
    currentReplicas: number;
    selector: Record<string, string>;
    onScale: (replicas: number) => Promise<void>;
    onClose: () => void;
}

export default function ScaleModal({
    resourceType,
    resourceName,
    namespace,
    currentReplicas,
    selector,
    onScale,
    onClose
}: ScaleModalProps) {
    const [pdbMinimum, setPdbMinimum] = useState<number | null>(null);
    const [pdbName, setPdbName] = useState<string>('');

    const form = useForm({
        schema: scaleSchema,
        initialValues: { replicas: currentReplicas },
        onSubmit: async (values) => {
            if (values.replicas === currentReplicas) {
                onClose();
                return;
            }
            await onScale(values.replicas);
            onClose();
        },
    });

    // Fetch PDBs on mount to check for constraints
    useEffect(() => {
        const checkPDBs = async () => {
            try {
                const pdbs = await ListPDBs('', namespace);

                // Find PDB that matches this workload's selector
                for (const pdb of pdbs) {
                    const pdbSelector = pdb.spec?.selector?.matchLabels || {};

                    // Check if PDB selector matches workload selector
                    const matches = Object.entries(pdbSelector).every(
                        ([key, value]) => selector[key] === value
                    );

                    if (matches && Object.keys(pdbSelector).length > 0) {
                        // Calculate minimum replicas from PDB
                        let minReplicas = 0;

                        if (pdb.spec?.minAvailable !== undefined) {
                            // Could be int or percentage string like "50%"
                            const minAvail = pdb.spec.minAvailable;
                            if (typeof minAvail === 'string' && minAvail.endsWith('%')) {
                                const percent = parseInt(minAvail);
                                minReplicas = Math.ceil((currentReplicas * percent) / 100);
                            } else {
                                minReplicas = parseInt(minAvail as any) || 0;
                            }
                        } else if (pdb.spec?.maxUnavailable !== undefined) {
                            // maxUnavailable means: currentReplicas - maxUnavailable = minimum
                            const maxUnav = pdb.spec.maxUnavailable;
                            if (typeof maxUnav === 'string' && maxUnav.endsWith('%')) {
                                const percent = parseInt(maxUnav);
                                const maxUnavCount = Math.floor((currentReplicas * percent) / 100);
                                minReplicas = currentReplicas - maxUnavCount;
                            } else {
                                const maxUnavCount = parseInt(maxUnav as any) || 0;
                                minReplicas = currentReplicas - maxUnavCount;
                            }
                        }

                        setPdbMinimum(minReplicas);
                        setPdbName(pdb.metadata?.name || 'unknown');
                        break; // Use first matching PDB
                    }
                }
            } catch (err: any) {
                // Silently fail - PDB check is optional
                console.error('Failed to check PDBs:', err);
            }
        };

        checkPDBs();
    }, [namespace, selector, currentReplicas]);

    const increment = () => form.setValue('replicas', Math.min(form.values.replicas + 1, 100));
    const decrement = () => form.setValue('replicas', Math.max(form.values.replicas - 1, 0));

    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
            <div
                className="bg-surface border border-border rounded-lg shadow-xl w-96 max-w-full m-4"
                onClick={(e) => e.stopPropagation()}
            >
                {/* Header */}
                <div className="px-6 py-4 border-b border-border">
                    <h2 className="text-lg font-semibold text-gray-200">Scale {resourceType}</h2>
                    <p className="text-sm text-gray-400 mt-1 selectable">
                        {namespace}/{resourceName}
                    </p>
                </div>

                {/* Body */}
                <div className="px-6 py-6">
                    <div className="space-y-4">
                        {/* Current replicas info */}
                        <div className="text-sm text-gray-400">
                            Current replicas: <span className="text-gray-200 font-medium">{currentReplicas}</span>
                        </div>

                        {/* Replica count control */}
                        <div className="space-y-2">
                            <label className="block text-sm text-gray-400">New replica count</label>
                            <div className="flex items-center gap-3">
                                <button
                                    onClick={decrement}
                                    disabled={form.values.replicas <= 0 || form.isSubmitting}
                                    className="p-2 rounded bg-surface-light border border-border hover:bg-surface-hover disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                                    title="Decrease replicas"
                                >
                                    <MinusIcon className="w-4 h-4 text-gray-300" />
                                </button>

                                <input
                                    type="number"
                                    value={form.values.replicas}
                                    onChange={(e: any) => form.setValue('replicas', Math.max(0, Math.min(100, parseInt(e.target.value) || 0)))}
                                    onBlur={() => form.setFieldTouched('replicas')}
                                    disabled={form.isSubmitting}
                                    className="flex-1 text-center text-2xl font-bold bg-background-dark border border-border rounded px-4 py-3 text-gray-200 focus:border-primary outline-none disabled:opacity-50"
                                    min="0"
                                    max="100"
                                />

                                <button
                                    onClick={increment}
                                    disabled={form.values.replicas >= 100 || form.isSubmitting}
                                    className="p-2 rounded bg-surface-light border border-border hover:bg-surface-hover disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                                    title="Increase replicas"
                                >
                                    <PlusIcon className="w-4 h-4 text-gray-300" />
                                </button>
                            </div>
                            <p className="text-xs text-gray-500">Valid range: 0-100 replicas</p>
                        </div>

                        {/* Change indicator */}
                        {form.values.replicas !== currentReplicas && (
                            <div className="text-sm text-center">
                                <span className={`font-medium ${form.values.replicas > currentReplicas ? 'text-green-400' : 'text-amber-400'}`}>
                                    {form.values.replicas > currentReplicas ? '↑' : '↓'} {Math.abs(form.values.replicas - currentReplicas)} replica
                                    {Math.abs(form.values.replicas - currentReplicas) !== 1 ? 's' : ''}
                                </span>
                            </div>
                        )}

                        {/* PDB warning when scaling below minimum */}
                        {pdbMinimum !== null && form.values.replicas < pdbMinimum && (
                            <div className="flex items-start gap-2 p-3 bg-amber-500/10 border border-amber-500/30 rounded text-xs text-amber-400">
                                <ExclamationTriangleIcon className="w-4 h-4 shrink-0 mt-0.5" />
                                <div>
                                    <strong>PDB Constraint:</strong> PodDisruptionBudget "{pdbName}" requires minimum {pdbMinimum} replica
                                    {pdbMinimum !== 1 ? 's' : ''}. Pods may not terminate below this threshold.
                                </div>
                            </div>
                        )}

                        {/* Error message */}
                        {form.submitError && (
                            <div className="p-3 bg-red-500/10 border border-red-500/30 rounded text-sm text-red-400">
                                {form.submitError}
                            </div>
                        )}
                    </div>
                </div>

                {/* Footer */}
                <div className="px-6 py-4 border-t border-border flex gap-3 justify-end">
                    <button
                        onClick={onClose}
                        disabled={form.isSubmitting}
                        className="px-4 py-2 text-sm rounded bg-surface-light border border-border hover:bg-surface-hover disabled:opacity-50 transition-colors"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={form.handleSubmit}
                        disabled={form.isSubmitting || form.values.replicas === currentReplicas}
                        className="px-4 py-2 text-sm rounded bg-primary hover:bg-primary/80 disabled:opacity-50 disabled:cursor-not-allowed text-white font-medium transition-colors"
                    >
                        {form.isSubmitting ? 'Scaling...' : 'Scale'}
                    </button>
                </div>
            </div>
        </div>
    );
}
