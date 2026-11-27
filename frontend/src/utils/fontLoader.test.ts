import { describe, it, expect, vi } from 'vitest';

vi.mock('../fonts/dm-sans.css', () => {
    (globalThis as any).__dmLoads = ((globalThis as any).__dmLoads || 0) + 1;
    return {};
});

describe('fontLoader', () => {
    it('loads DM Sans only when selected, and only once', async () => {
        const { ensureFontsForFamily, startFontLoading } = await import('./fontLoader');
        ensureFontsForFamily("'Inter', system-ui, sans-serif");
        await new Promise(r => setTimeout(r, 10));
        expect((globalThis as any).__dmLoads || 0).toBe(0);
        ensureFontsForFamily("'DM Sans', system-ui, sans-serif");
        ensureFontsForFamily("'DM Sans', system-ui, sans-serif");
        await new Promise(r => setTimeout(r, 10));
        expect((globalThis as any).__dmLoads).toBe(1);
        // No MutationObserver in the node environment: must be a safe no-op.
        const root: any = { style: { getPropertyValue: () => '' } };
        expect(() => startFontLoading(root)()).not.toThrow();
    });
});
