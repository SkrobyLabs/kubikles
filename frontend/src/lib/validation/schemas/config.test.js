import { describe, it, expect } from 'vitest';
import { appConfigSchema } from './config';

describe('sidebar layout validation', () => {
    it('accepts valid sidebar layout config', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [
                        { id: 'workloads', title: 'Workloads', items: ['pods', 'deployments'] },
                        { id: 'network', title: 'Network', items: ['services'] },
                    ],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('accepts config with empty sidebar object', () => {
        const config = { ui: { sidebar: {} } };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('accepts config with undefined layout', () => {
        const config = { ui: { sidebar: { layout: undefined } } };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('accepts layout section with itemLabels', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [
                        {
                            id: 'workloads',
                            title: 'Workloads',
                            items: ['pods'],
                            itemLabels: { pods: 'My Pods' },
                        },
                    ],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('accepts layout section with isCustom flag', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [
                        { id: 'my-section', title: 'My Section', items: [], isCustom: true },
                    ],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('rejects layout section missing id', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [{ title: 'Test', items: ['pods'] }],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(false);
    });

    it('rejects layout section missing title', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [{ id: 'test', items: ['pods'] }],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(false);
    });

    it('rejects layout section missing items array', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [{ id: 'test', title: 'Test' }],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(false);
    });

    it('rejects non-string items in items array', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [{ id: 'test', title: 'Test', items: [123] }],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(false);
    });

    it('rejects non-string values in itemLabels', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [
                        { id: 'test', title: 'Test', items: ['pods'], itemLabels: { pods: 123 } },
                    ],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(false);
    });

    it('accepts excludedItems as string array', () => {
        const config = {
            ui: {
                sidebar: {
                    excludedItems: ['pods', 'services'],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('accepts empty excludedItems array', () => {
        const config = {
            ui: {
                sidebar: {
                    excludedItems: [],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('accepts undefined excludedItems', () => {
        const config = {
            ui: {
                sidebar: {
                    excludedItems: undefined,
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });

    it('rejects non-string values in excludedItems', () => {
        const config = {
            ui: {
                sidebar: {
                    excludedItems: [123],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(false);
    });

    it('accepts layout and excludedItems together', () => {
        const config = {
            ui: {
                sidebar: {
                    layout: [
                        { id: 'workloads', title: 'Workloads', items: ['pods'] },
                    ],
                    excludedItems: ['services', 'nodes'],
                },
            },
        };
        const result = appConfigSchema.safeParse(config);
        expect(result.success).toBe(true);
    });
});

describe('ai commandAllowlist validation', () => {
    it('accepts valid commandAllowlist as string array', () => {
        const result = appConfigSchema.safeParse({
            ai: { commandAllowlist: ['kubectl get', 'helm list'] },
        });
        expect(result.success).toBe(true);
    });

    it('accepts empty commandAllowlist', () => {
        const result = appConfigSchema.safeParse({
            ai: { commandAllowlist: [] },
        });
        expect(result.success).toBe(true);
    });

    it('accepts omitted commandAllowlist', () => {
        const result = appConfigSchema.safeParse({ ai: {} });
        expect(result.success).toBe(true);
    });

    it('rejects non-string values in commandAllowlist', () => {
        const result = appConfigSchema.safeParse({
            ai: { commandAllowlist: [123] },
        });
        expect(result.success).toBe(false);
    });

    it('rejects non-array commandAllowlist', () => {
        const result = appConfigSchema.safeParse({
            ai: { commandAllowlist: 'kubectl get' },
        });
        expect(result.success).toBe(false);
    });

    it('accepts commandAllowlist alongside allowedTools', () => {
        const result = appConfigSchema.safeParse({
            ai: {
                allowedTools: ['run_command'],
                commandAllowlist: ['kubectl get', 'kubectl logs'],
            },
        });
        expect(result.success).toBe(true);
    });
});

describe('debug config validation', () => {
    it('accepts showDebugIcon as true', () => {
        const result = appConfigSchema.safeParse({ debug: { showDebugIcon: true } });
        expect(result.success).toBe(true);
    });

    it('accepts showDebugIcon as false', () => {
        const result = appConfigSchema.safeParse({ debug: { showDebugIcon: false } });
        expect(result.success).toBe(true);
    });

    it('accepts omitted showDebugIcon (optional)', () => {
        const result = appConfigSchema.safeParse({ debug: {} });
        expect(result.success).toBe(true);
    });

    it('rejects non-boolean showDebugIcon', () => {
        const result = appConfigSchema.safeParse({ debug: { showDebugIcon: 'yes' } });
        expect(result.success).toBe(false);
    });

    it('accepts showLogSourceMarkers alongside showDebugIcon', () => {
        const result = appConfigSchema.safeParse({
            debug: { showDebugIcon: true, showLogSourceMarkers: true },
        });
        expect(result.success).toBe(true);
    });
});
