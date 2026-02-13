import React, { createContext, useContext, useState, useEffect, useMemo, useCallback } from 'react';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { GetCurrentTheme, GetThemes, SetTheme } from 'wailsjs/go/main/App';
import { main } from 'wailsjs/go/models';

// Type aliases for clarity
type Theme = main.Theme;
type ThemeList = main.Theme[];

interface FontOption {
    id: string;
    name: string;
    family: string;
}

interface ThemeContextValue {
    currentTheme: Theme | null;
    themes: ThemeList;
    loading: boolean;
    switchTheme: (themeId: string) => Promise<void>;
    refreshThemes: () => Promise<void>;
    uiFont: string;
    monoFont: string;
    setUiFont: (fontId: string) => void;
    setMonoFont: (fontId: string) => void;
    uiFonts: FontOption[];
    monoFonts: FontOption[];
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined);

// Available UI fonts (sans-serif for interface)
// - Inter: Modern, highly legible, designed for screens (bundled)
// - DM Sans: Slightly condensed, geometric, rounded (bundled)
// - SF Pro: Apple's system font, native macOS feel
// - Segoe UI: Microsoft's system font, clean and professional
// - System Default: Uses the OS default sans-serif font
export const UI_FONTS: FontOption[] = [
    { id: 'inter', name: 'Inter', family: "'Inter', system-ui, sans-serif" },
    { id: 'dm-sans', name: 'DM Sans', family: "'DM Sans', system-ui, sans-serif" },
    { id: 'sf-pro', name: 'SF Pro', family: "-apple-system, BlinkMacSystemFont, 'SF Pro Text', sans-serif" },
    { id: 'segoe', name: 'Segoe UI', family: "'Segoe UI', Roboto, sans-serif" },
    { id: 'system', name: 'System Default', family: "system-ui, -apple-system, BlinkMacSystemFont, sans-serif" },
];

// Available monospace fonts (for code, logs, terminals)
// - JetBrains Mono: Excellent for code, ligatures support (bundled)
// - SF Mono: Apple's monospace font, clean and consistent
// - Menlo: Classic macOS monospace, very readable
// - Consolas: Microsoft's monospace, great for code
// - Fira Code: Popular coding font with ligatures
export const MONO_FONTS: FontOption[] = [
    { id: 'jetbrains', name: 'JetBrains Mono', family: "'JetBrains Mono', monospace" },
    { id: 'sf-mono', name: 'SF Mono', family: "'SF Mono', SFMono-Regular, monospace" },
    { id: 'menlo', name: 'Menlo', family: "'Menlo', Monaco, monospace" },
    { id: 'consolas', name: 'Consolas', family: "'Consolas', 'Courier New', monospace" },
    { id: 'fira', name: 'Fira Code', family: "'Fira Code', 'Fira Mono', monospace" },
];

const FONT_STORAGE_KEY = 'kubikles-fonts';

interface FontPreferences {
    ui: string;
    mono: string;
}

export const useTheme = (): ThemeContextValue => {
    const context = useContext(ThemeContext);
    if (!context) {
        throw new Error('useTheme must be used within a ThemeProvider');
    }
    return context;
};

// Convert hex color (#rrggbb or #rgb) to "R G B" space-separated string for CSS variables
const hexToRgbTriplet = (hex: string): string | null => {
    if (!hex) return null;
    const cleaned = hex.replace('#', '');
    let r: number, g: number, b: number;
    if (cleaned.length === 3) {
        r = parseInt(cleaned[0] + cleaned[0], 16);
        g = parseInt(cleaned[1] + cleaned[1], 16);
        b = parseInt(cleaned[2] + cleaned[2], 16);
    } else if (cleaned.length === 6) {
        r = parseInt(cleaned.slice(0, 2), 16);
        g = parseInt(cleaned.slice(2, 4), 16);
        b = parseInt(cleaned.slice(4, 6), 16);
    } else {
        return null;
    }
    return `${r} ${g} ${b}`;
};

// Apply theme colors to CSS variables
const applyThemeColors = (theme: Theme | null): void => {
    if (!theme || !theme.colors) return;

    const root = document.documentElement;
    const colors = theme.colors;

    // Set theme type attribute for CSS overrides (light/dark)
    root.setAttribute('data-theme-type', theme.type || 'dark');

    // Apply color variables
    root.style.setProperty('--color-background', colors.background || '#1e1e1e');
    root.style.setProperty('--color-background-dark', colors.backgroundDark || '#1a1a1a');
    root.style.setProperty('--color-surface', colors.surface || '#252526');
    root.style.setProperty('--color-surface-light', colors.surfaceLight || '#2d2d2d');
    root.style.setProperty('--color-surface-hover', colors.surfaceHover || '#3d3d3d');
    root.style.setProperty('--color-primary', colors.primary || '#007acc');
    root.style.setProperty('--color-text', colors.text || '#cccccc');
    root.style.setProperty('--color-text-muted', colors.textMuted || '#808080');
    root.style.setProperty('--color-border', colors.border || '#3e3e42');
    root.style.setProperty('--color-border-rgb', hexToRgbTriplet(colors.border || '#3e3e42') || '62 62 66');
    root.style.setProperty('--color-success', colors.success || '#4CC38A');
    root.style.setProperty('--color-success-dark', colors.successDark || '#3AA876');
    root.style.setProperty('--color-error', colors.error || '#E5484D');
    root.style.setProperty('--color-error-dark', colors.errorDark || '#C33A3F');
    root.style.setProperty('--color-warning', colors.warning || '#F5A623');
    root.style.setProperty('--color-warning-dark', colors.warningDark || '#D98C1C');
    root.style.setProperty('--color-red-orange', colors.redOrange || '#E66B2F');
    root.style.setProperty('--color-red-orange-dark', colors.redOrangeDark || '#C75A27');
    root.style.setProperty('--color-scrollbar-track', colors.scrollbarTrack || '#252526');
    root.style.setProperty('--color-scrollbar-thumb', colors.scrollbarThumb || '#3e3e42');
    root.style.setProperty('--color-scrollbar-thumb-hover', colors.scrollbarThumbHover || '#007acc');

    // Apply gray palette if theme provides one (otherwise CSS defaults apply)
    const grayShades: [string, string | undefined][] = [
        ['50', colors.gray50], ['100', colors.gray100], ['200', colors.gray200],
        ['300', colors.gray300], ['400', colors.gray400], ['500', colors.gray500],
        ['600', colors.gray600], ['700', colors.gray700], ['800', colors.gray800],
        ['900', colors.gray900], ['950', colors.gray950],
    ];
    for (const [shade, hex] of grayShades) {
        if (hex) {
            const rgb = hexToRgbTriplet(hex);
            if (rgb) {
                root.style.setProperty(`--gray-${shade}`, rgb);
            }
        } else {
            // Remove override so CSS default applies
            root.style.removeProperty(`--gray-${shade}`);
        }
    }
};

// Apply font CSS variables
const applyFonts = (uiFontId: string, monoFontId: string): void => {
    const root = document.documentElement;
    const uiFont = UI_FONTS.find((f: any) => f.id === uiFontId) || UI_FONTS[0];
    const monoFont = MONO_FONTS.find((f: any) => f.id === monoFontId) || MONO_FONTS[0];

    root.style.setProperty('--font-ui', uiFont.family);
    root.style.setProperty('--font-mono', monoFont.family);
};

// Load saved font preferences
const loadFontPreferences = (): FontPreferences => {
    try {
        const saved = localStorage.getItem(FONT_STORAGE_KEY);
        if (saved) {
            return JSON.parse(saved) as FontPreferences;
        }
    } catch (e: any) {
        console.error('Failed to load font preferences:', e);
    }
    return { ui: 'inter', mono: 'jetbrains' };
};

// Save font preferences
const saveFontPreferences = (ui: string, mono: string): void => {
    try {
        localStorage.setItem(FONT_STORAGE_KEY, JSON.stringify({ ui, mono }));
    } catch (e: any) {
        console.error('Failed to save font preferences:', e);
    }
};

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [currentTheme, setCurrentTheme] = useState<Theme | null>(null);
    const [themes, setThemes] = useState<ThemeList>([]);
    const [loading, setLoading] = useState<boolean>(true);

    // Font preferences
    const [uiFont, setUiFontState] = useState<string>('inter');
    const [monoFont, setMonoFontState] = useState<string>('jetbrains');

    // Load initial theme, fonts, and theme list
    useEffect(() => {
        const loadInitialTheme = async () => {
            try {
                // Load font preferences first
                const fontPrefs = loadFontPreferences();
                setUiFontState(fontPrefs.ui);
                setMonoFontState(fontPrefs.mono);
                applyFonts(fontPrefs.ui, fontPrefs.mono);

                const [theme, themeList] = await Promise.all([
                    GetCurrentTheme(),
                    GetThemes()
                ]);

                if (theme) {
                    setCurrentTheme(theme);
                    applyThemeColors(theme);
                }

                if (themeList) {
                    setThemes(themeList);
                }
            } catch (err: any) {
                console.error('Failed to load theme:', err);
            } finally {
                setLoading(false);
            }
        };

        loadInitialTheme();
    }, []);

    // Listen for theme changes from Go backend (via menu)
    useEffect(() => {
        const handleThemeChanged = (theme: Theme) => {
            setCurrentTheme(theme);
            applyThemeColors(theme);
        };

        const handleThemeListChanged = (themeList: ThemeList) => {
            setThemes(themeList);
        };

        const unsubThemeChanged = EventsOn('theme:changed', handleThemeChanged);
        const unsubListChanged = EventsOn('theme:list-changed', handleThemeListChanged);

        return () => {
            if (typeof unsubThemeChanged === 'function') unsubThemeChanged();
            if (typeof unsubListChanged === 'function') unsubListChanged();
        };
    }, []);

    // Switch theme programmatically
    const switchTheme = useCallback(async (themeId: string) => {
        try {
            await SetTheme(themeId);
            // Theme will be applied via the event listener
        } catch (err: any) {
            console.error('Failed to switch theme:', err);
        }
    }, []);

    // Refresh theme list (after user adds themes)
    const refreshThemes = useCallback(async () => {
        try {
            const themeList = await GetThemes();
            if (themeList) {
                setThemes(themeList);
            }
        } catch (err: any) {
            console.error('Failed to refresh themes:', err);
        }
    }, []);

    // Set UI font
    const setUiFont = useCallback((fontId: string) => {
        setUiFontState(fontId);
        applyFonts(fontId, monoFont);
        saveFontPreferences(fontId, monoFont);
    }, [monoFont]);

    // Set monospace font
    const setMonoFont = useCallback((fontId: string) => {
        setMonoFontState(fontId);
        applyFonts(uiFont, fontId);
        saveFontPreferences(uiFont, fontId);
    }, [uiFont]);

    const value: ThemeContextValue = useMemo(() => ({
        currentTheme,
        themes,
        loading,
        switchTheme,
        refreshThemes,
        // Font selection
        uiFont,
        monoFont,
        setUiFont,
        setMonoFont,
        uiFonts: UI_FONTS,
        monoFonts: MONO_FONTS,
    }), [currentTheme, themes, loading, switchTheme, refreshThemes, uiFont, monoFont, setUiFont, setMonoFont]);

    return (
        <ThemeContext.Provider value={value}>
            {children}
        </ThemeContext.Provider>
    );
};
