/// <reference types="vitest" />
import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react-swc'
import {viteStaticCopy} from 'vite-plugin-static-copy'
import {visualizer} from 'rollup-plugin-visualizer'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  resolve: {
    alias: {
      // Path alias for cleaner imports (e.g., ~/hooks instead of ../../../hooks)
      '~': path.resolve(__dirname, 'src'),
      // Redirect wailsjs imports to our adapters for dual-mode support
      // The adapters detect runtime mode and route to Wails bindings or HTTP API
      'wailsjs/go/main/App': path.resolve(__dirname, 'src/lib/wailsjs-adapter/go/main/App.ts'),
      'wailsjs/go/models': path.resolve(__dirname, 'wailsjs/go/models.ts'),
      'wailsjs/runtime/runtime': path.resolve(__dirname, 'src/lib/wailsjs-adapter/runtime/runtime.ts'),
      // Also handle relative imports that may occur
      '../wailsjs/go/main/App': path.resolve(__dirname, 'src/lib/wailsjs-adapter/go/main/App.ts'),
      '../../wailsjs/go/main/App': path.resolve(__dirname, 'src/lib/wailsjs-adapter/go/main/App.ts'),
      '../../../wailsjs/go/main/App': path.resolve(__dirname, 'src/lib/wailsjs-adapter/go/main/App.ts'),
      '../../../../wailsjs/go/main/App': path.resolve(__dirname, 'src/lib/wailsjs-adapter/go/main/App.ts'),
      '../wailsjs/runtime/runtime': path.resolve(__dirname, 'src/lib/wailsjs-adapter/runtime/runtime.ts'),
      '../../wailsjs/runtime/runtime': path.resolve(__dirname, 'src/lib/wailsjs-adapter/runtime/runtime.ts'),
      '../../../wailsjs/runtime/runtime': path.resolve(__dirname, 'src/lib/wailsjs-adapter/runtime/runtime.ts'),
      '../../../../wailsjs/runtime/runtime': path.resolve(__dirname, 'src/lib/wailsjs-adapter/runtime/runtime.ts'),
    },
  },
  test: {
    globals: true,
    environment: 'node',
  },
  // The view worker is built as an inline Blob (?worker&inline) so loading it never depends on
  // the asset scheme handler. Classic (iife) format works on older WKWebView.
  worker: {
    format: 'iife',
  },
  build: {
    rollupOptions: {
      output: {
        // Function form so shared deps are assigned deliberately. With the
        // object form React ended up inside the flow chunk, which made the
        // entry modulepreload React Flow and Monaco before first paint.
        // flow / monaco / xterm are only reachable from lazy imports.
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (/node_modules\/(react|react-dom|scheduler)\//.test(id)) return 'vendor-react'
          // main.tsx imports @monaco-editor/loader to point it at local files, so
          // it is reachable at startup. Give it its own tiny chunk so the editor
          // wrapper (monaco) stays lazy-only.
          if (/node_modules\/(@monaco-editor\/loader|state-local)\//.test(id)) return 'monaco-loader'
          if (/node_modules\/(monaco-editor|@monaco-editor\/react)\//.test(id)) return 'monaco'
          if (/node_modules\/(@xyflow|dagre|@dagrejs|graphlib)\//.test(id)) return 'flow'
          if (/node_modules\/(xterm|xterm-addon-fit|@xterm)\//.test(id)) return 'xterm'
          return undefined
        },
      }
    }
  },
  plugins: [
    react(),
    ...(process.env.ANALYZE ? [visualizer({
      filename: 'dist/stats.html',
      open: true,
      gzipSize: true,
      brotliSize: true,
    })] : []),
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
