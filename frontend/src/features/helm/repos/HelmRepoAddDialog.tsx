import React from 'react';
import { XMarkIcon } from '@heroicons/react/24/outline';
import { AddHelmRepository } from 'wailsjs/go/main/App';
import { useNotification } from '~/context';
import { useForm, helmRepoSchema } from '~/lib/validation';

export default function HelmRepoAddDialog({ onClose, onSuccess }) {
    const { addNotification } = useNotification();

    const form = useForm({
        schema: helmRepoSchema,
        initialValues: { name: '', url: '', priority: 100 },
        onSubmit: async (values) => {
            await AddHelmRepository(values.name.trim(), values.url.trim(), values.priority);
            addNotification({
                type: 'success',
                title: 'Repository added',
                message: `Successfully added ${values.name.trim()}`
            });
            onSuccess();
        },
    });

    const popularRepos = [
        { name: 'bitnami', url: 'https://charts.bitnami.com/bitnami' },
        { name: 'prometheus-community', url: 'https://prometheus-community.github.io/helm-charts' },
        { name: 'grafana', url: 'https://grafana.github.io/helm-charts' },
        { name: 'jetstack', url: 'https://charts.jetstack.io' },
        { name: 'ingress-nginx', url: 'https://kubernetes.github.io/ingress-nginx' },
    ];

    const handleQuickAdd = (repo) => {
        form.setValues({ name: repo.name, url: repo.url });
    };

    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-surface border border-border rounded-lg shadow-xl w-full max-w-lg">
                {/* Header */}
                <div className="flex items-center justify-between px-6 py-4 border-b border-border">
                    <h2 className="text-lg font-semibold">Add Helm Repository</h2>
                    <button
                        onClick={onClose}
                        className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                    >
                        <XMarkIcon className="h-5 w-5" />
                    </button>
                </div>

                {/* Form */}
                <form onSubmit={form.handleSubmit} className="p-6 space-y-4">
                    {/* Quick Add */}
                    <div>
                        <label className="block text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">
                            Quick Add
                        </label>
                        <div className="flex flex-wrap gap-2">
                            {popularRepos.map((repo) => (
                                <button
                                    key={repo.name}
                                    type="button"
                                    onClick={() => handleQuickAdd(repo)}
                                    className="px-2 py-1 text-xs bg-white/5 hover:bg-white/10 border border-border rounded transition-colors"
                                >
                                    {repo.name}
                                </button>
                            ))}
                        </div>
                    </div>

                    {/* Name */}
                    <div>
                        <label htmlFor="repo-name" className="block font-medium text-gray-300 mb-1">
                            Repository Name
                        </label>
                        <input
                            id="repo-name"
                            type="text"
                            {...form.getFieldProps('name')}
                            placeholder="e.g., my-repo"
                            className="w-full"
                            disabled={form.isSubmitting}
                        />
                        {form.touched.name && form.errors.name && (
                            <span className="text-red-400 text-xs mt-1 block">{form.errors.name}</span>
                        )}
                    </div>

                    {/* URL */}
                    <div>
                        <label htmlFor="repo-url" className="block font-medium text-gray-300 mb-1">
                            Repository URL
                        </label>
                        <input
                            id="repo-url"
                            type="text"
                            {...form.getFieldProps('url')}
                            placeholder="https://charts.example.com"
                            className="w-full"
                            disabled={form.isSubmitting}
                        />
                        {form.touched.url && form.errors.url && (
                            <span className="text-red-400 text-xs mt-1 block">{form.errors.url}</span>
                        )}
                    </div>

                    {/* Priority */}
                    <div>
                        <label htmlFor="repo-priority" className="block font-medium text-gray-300 mb-1">
                            Priority
                            <span className="text-xs text-gray-500 ml-2">(Lower number = higher priority)</span>
                        </label>
                        <input
                            id="repo-priority"
                            type="number"
                            min="0"
                            value={form.values.priority}
                            onChange={(e) => form.setValue('priority', parseInt(e.target.value) || 0)}
                            onBlur={() => form.setFieldTouched('priority')}
                            className="w-24"
                            disabled={form.isSubmitting}
                        />
                        {form.touched.priority && form.errors.priority && (
                            <span className="text-red-400 text-xs mt-1 block">{form.errors.priority}</span>
                        )}
                    </div>

                    {/* Error */}
                    {form.submitError && (
                        <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-md text-red-400 text-sm">
                            {form.submitError}
                        </div>
                    )}

                    {/* Actions */}
                    <div className="flex justify-end gap-3 pt-4">
                        <button
                            type="button"
                            onClick={onClose}
                            disabled={form.isSubmitting}
                            className="px-4 py-2 text-sm text-gray-400 hover:text-white hover:bg-white/10 rounded-md transition-colors disabled:opacity-50"
                        >
                            Cancel
                        </button>
                        <button
                            type="submit"
                            disabled={form.isSubmitting || !form.isValid}
                            className="px-4 py-2 text-sm bg-primary hover:bg-primary/80 text-white rounded-md transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                        >
                            {form.isSubmitting && (
                                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
                            )}
                            Add Repository
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
}
