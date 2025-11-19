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
      }
    },
  },
  plugins: [],
}
