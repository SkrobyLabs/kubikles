/**
 * Bundled fonts: Inter and JetBrains Mono are the defaults and load eagerly
 * (src/fonts/core.css). DM Sans is an optional UI font, so its files are only
 * fetched once it is actually selected. ThemeContext applies a font choice by
 * writing the --font-ui / --font-mono custom properties on <html>; watching
 * that is enough, so no cross-module call is needed.
 */

const lazyFonts: { match: RegExp; load: () => Promise<unknown> }[] = [
    { match: /dm sans/i, load: () => import('../fonts/dm-sans.css') },
];

const requested = new Set<number>();

export function ensureFontsForFamily(family: string): void {
    lazyFonts.forEach((font, i) => {
        if (requested.has(i) || !font.match.test(family)) return;
        requested.add(i);
        font.load().catch(() => requested.delete(i));
    });
}

function readFamilies(root: HTMLElement): string {
    return `${root.style.getPropertyValue('--font-ui')} ${root.style.getPropertyValue('--font-mono')}`;
}

export function startFontLoading(root: HTMLElement = document.documentElement): () => void {
    ensureFontsForFamily(readFamilies(root));
    if (typeof MutationObserver === 'undefined') return () => {};
    const observer = new MutationObserver(() => ensureFontsForFamily(readFamilies(root)));
    observer.observe(root, { attributes: true, attributeFilter: ['style'] });
    return () => observer.disconnect();
}
