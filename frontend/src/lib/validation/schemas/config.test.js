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
});
