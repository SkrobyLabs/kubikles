/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // Theme colors via CSS variables (allows runtime switching)
        background: {
          DEFAULT: 'var(--color-background)',
          dark: 'var(--color-background-dark)',
        },
        surface: {
          DEFAULT: 'var(--color-surface)',
          light: 'var(--color-surface-light)',
          hover: 'var(--color-surface-hover)',
        },
        primary: 'var(--color-primary)',
        text: 'var(--color-text)',
        'text-muted': 'var(--color-text-muted)',
        border: 'var(--color-border)',

        // Status Colors
        success: {
          DEFAULT: 'var(--color-success)',
          dark: 'var(--color-success-dark)',
        },
        error: {
          DEFAULT: 'var(--color-error)',
          dark: 'var(--color-error-dark)',
        },
        warning: {
          DEFAULT: 'var(--color-warning)',
          dark: 'var(--color-warning-dark)',
        },
        'red-orange': {
          DEFAULT: 'var(--color-red-orange)',
          dark: 'var(--color-red-orange-dark)',
        },
      },
      fontFamily: {
        sans: ['var(--font-ui)'],
        mono: ['var(--font-mono)'],
      },
    },
  },
  safelist: [
    'text-success',
    'text-success/70',
    'text-error',
    'text-warning',
    'text-red-orange',
    'bg-success',
    'bg-success/50',
    'bg-error',
    'bg-warning',
    'bg-red-orange',
    'bg-surface',
    'bg-surface-light',
    'bg-surface-hover',
    'bg-background',
    'bg-background-dark',
    'text-text-muted',
    'hover:bg-surface-hover',
  ],
  plugins: [],
}
