/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // Premium dark theme colors
        background: '#1e1e1e',
        surface: '#252526',
        primary: '#007acc',
        text: '#cccccc',
        border: '#3e3e42',

        // Status Colors
        success: {
          DEFAULT: '#4CC38A',
          dark: '#3AA876',
        },
        error: {
          DEFAULT: '#E5484D',
          dark: '#C33A3F',
        },
        warning: {
          DEFAULT: '#F5A623',
          dark: '#D98C1C',
        },
        'red-orange': {
          DEFAULT: '#E66B2F',
          dark: '#C75A27',
        },
      }
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
  ],
  plugins: [],
}
