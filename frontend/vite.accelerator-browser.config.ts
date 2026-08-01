import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { browserIsolationPlugin, validateBuildVersion } from './scripts/accelerator-browser-artifact.mjs';

const frontendRoot = path.dirname(fileURLToPath(import.meta.url));
const buildVersion = validateBuildVersion(process.env.BUILD_VERSION);

export default defineConfig({
  base: '/accelerator/browser/',
  publicDir: false,
  define: { 'process.env.NODE_ENV': JSON.stringify('production') },
  resolve: { alias: { '~': path.resolve(frontendRoot, 'src') } },
  plugins: [react(), browserIsolationPlugin(frontendRoot, buildVersion)],
  build: {
    outDir: path.resolve(frontendRoot, 'dist/accelerator-browser'),
    emptyOutDir: true,
    copyPublicDir: false,
    cssCodeSplit: false,
    sourcemap: false,
    manifest: false,
    lib: {
      entry: path.resolve(frontendRoot, 'src/accelerator-browser/entry.tsx'),
      formats: ['es'],
      name: 'KubiklesAcceleratorBrowser',
    },
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
        entryFileNames: 'assets/browser.js',
        assetFileNames: asset => asset.name?.endsWith('.css') ? 'assets/browser.css' : 'assets/[name][extname]',
      },
    },
  },
});
