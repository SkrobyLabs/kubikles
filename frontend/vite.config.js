import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import {viteStaticCopy} from 'vite-plugin-static-copy'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    react(),
    viteStaticCopy({
      targets: [
        // Core Monaco loader
        {
          src: 'node_modules/monaco-editor/min/vs/loader.js',
          dest: 'monaco-editor/min/vs'
        },
        // Base workers
        {
          src: 'node_modules/monaco-editor/min/vs/base',
          dest: 'monaco-editor/min/vs'
        },
        // Core editor (required)
        {
          src: 'node_modules/monaco-editor/min/vs/editor',
          dest: 'monaco-editor/min/vs'
        },
        // YAML syntax highlighting
        {
          src: 'node_modules/monaco-editor/min/vs/basic-languages/yaml',
          dest: 'monaco-editor/min/vs/basic-languages'
        },
        // JSON language support (for schema validation)
        {
          src: 'node_modules/monaco-editor/min/vs/language/json',
          dest: 'monaco-editor/min/vs/language'
        }
      ]
    })
  ]
})
