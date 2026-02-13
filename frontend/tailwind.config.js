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
        border: 'rgb(var(--color-border-rgb) / <alpha-value>)',

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

        // Gray palette via CSS variables (enables light/dark theme switching)
        // Uses RGB triplet format for Tailwind opacity modifier support
        gray: {
          50:  'rgb(var(--gray-50) / <alpha-value>)',
          100: 'rgb(var(--gray-100) / <alpha-value>)',
          200: 'rgb(var(--gray-200) / <alpha-value>)',
          300: 'rgb(var(--gray-300) / <alpha-value>)',
          400: 'rgb(var(--gray-400) / <alpha-value>)',
          500: 'rgb(var(--gray-500) / <alpha-value>)',
          600: 'rgb(var(--gray-600) / <alpha-value>)',
          700: 'rgb(var(--gray-700) / <alpha-value>)',
          800: 'rgb(var(--gray-800) / <alpha-value>)',
          900: 'rgb(var(--gray-900) / <alpha-value>)',
          950: 'rgb(var(--gray-950) / <alpha-value>)',
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
