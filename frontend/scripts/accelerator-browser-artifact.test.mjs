import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { build } from 'vite';
import { ARTIFACT_FILES, assertAllowedModuleId, assertAllowedSource, browserIsolationPlugin, markerBytes, validateBuildVersion, verifyArtifact } from './accelerator-browser-artifact.mjs';

const roots = [];
afterEach(async () => Promise.all(roots.splice(0).map(root => rm(root, { recursive: true, force: true }))));

describe('Accelerator Browser artifact contract', () => {
  it('validates literal versions and canonical marker bytes', () => {
    expect(validateBuildVersion('v1.2.3')).toBe('v1.2.3');
    for (const value of [undefined, '', ' ', ' v1', 'v1 ', 'v1\n', 'v1\u0000', 'v1\u0085', 'v1\u009f']) expect(() => validateBuildVersion(value)).toThrow('BUILD_VERSION');
    expect(markerBytes('v1.2.3')).toBe('{"schemaVersion":1,"kind":"kubikles-accelerator-browser","buildVersion":"v1.2.3"}\n');
  });

  it('enforces source and exact output isolation', async () => {
    const frontend = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
    expect(() => assertAllowedModuleId(path.join(frontend, 'src/accelerator-browser/entry.tsx'), frontend)).not.toThrow();
    for (const forbidden of [
      'src/components/shared/SecretEditor.tsx',
      'src/components/shared/ResourceList.tsx',
      'src/context/K8sContext.tsx',
      'src/lib/wailsjs-adapter/go/main/App.ts',
      'src/features/helm/releases/HelmReleaseList.tsx',
      'src/components/shared/Terminal.tsx',
    ]) expect(() => assertAllowedModuleId(path.join(frontend, forbidden), frontend)).toThrow('Forbidden');
    expect(() => assertAllowedModuleId('\0hostile-virtual', frontend)).toThrow('virtual');
    expect(() => assertAllowedModuleId(path.join(frontend, 'node_modules/.vite/deps/hostile.js'), frontend)).toThrow('prebundle');
    expect(() => assertAllowedSource("const storageLabel = 'storage';", 'allowed.ts')).not.toThrow();
    for (const allowed of [
      'const fetch = value => value; fetch("/x")',
      'function local(window) { return window.fetch("/x"); }',
      'const labels = { fetch: "fetch", process: "process" };',
      'const process = { env: {} }; process.env.NODE_ENV;',
    ]) expect(() => assertAllowedSource(allowed, 'allowed.ts')).not.toThrow();
    for (const allowed of [
      `export function mountAcceleratorBrowser() {
        for (let localStorage = 0; localStorage < 1; localStorage += 1) void localStorage;
        for (const sessionStorage in { one: true }) void sessionStorage;
        for (const indexedDB of [1]) void indexedDB;
      }`,
      `export function mountAcceleratorBrowser(value) {
        switch (value) {
          case 0: { const caches = { open: () => 'local' }; return caches.open(); }
          case 1: class CacheStorage {}; return new CacheStorage();
          default: { const serviceWorker = { register: () => 'local' }; return serviceWorker.register(); }
        }
      }`,
      `export function mountAcceleratorBrowser() {
        for (var localStorage = 0; localStorage < 1; localStorage += 1) {}
        return localStorage;
      }`,
      `// localStorage sessionStorage indexedDB caches CacheStorage serviceWorker
       const message = 'navigator.storage and navigator.serviceWorker';
       const labels = {
         localStorage: message, sessionStorage: true, indexedDB() {},
         get caches() { return 'cache label'; }, serviceWorker() {},
       };
       class Labels { localStorage() {}; serviceWorker() {} }
       export function mountAcceleratorBrowser() { return [labels.localStorage, labels.caches, Labels]; }`,
      `const localStorage = { getItem: () => 'local' };
       const sessionStorage = { setItem: () => 'local' };
       class indexedDB { static open() { return 'local'; } }
       const caches = { open: () => 'local' };
       class CacheStorage {}
       const navigator = { storage: {}, serviceWorker: {} };
       const serviceWorker = { register: () => 'local' };
       export function mountAcceleratorBrowser() {
         return [localStorage.getItem(), sessionStorage.setItem(), indexedDB.open(), caches.open(),
           new CacheStorage(), navigator.storage, navigator.serviceWorker, serviceWorker.register()];
       }`,
      `export function mountAcceleratorBrowser(window, self, globalThis, navigator, capability) {
         return [window[capability], self[capability], globalThis[capability], navigator[capability]];
       }`,
    ]) expect(() => assertAllowedSource(allowed, 'lexical-shadow.ts')).not.toThrow();
    for (const transport of [
      'fetch("/x")',
      'new WebSocket("wss://x")',
      'globalThis.fetch("/x")',
      'window["WebSocket"]("wss://x")',
      'new self[`XMLHttpRequest`]()',
      'const scope = globalThis; const Open = scope["Event" + "Source"]; new Open("/x")',
      'const { fetch: send } = window; send("/x")',
    ]) {
      expect(() => assertAllowedSource(transport, 'hostile.ts')).toThrow('transport');
    }
    for (const nodeGlobal of ['process.env.NODE_ENV', 'Buffer.from("x")', 'require("x")', 'module.exports', 'global.process']) {
      expect(() => assertAllowedSource(nodeGlobal, 'hostile.ts')).toThrow('Node global');
    }
    for (const source of [
      'export function mountAcceleratorBrowser() { return localStorage.getItem("key"); }',
      'export function mountAcceleratorBrowser() { return sessionStorage["setItem"]("key", "value"); }',
      'export function mountAcceleratorBrowser() { return indexedDB.open("db"); }',
      'export function mountAcceleratorBrowser() { return caches.open("cache"); }',
      'export function mountAcceleratorBrowser() { return new CacheStorage(); }',
      'export function mountAcceleratorBrowser() { return navigator.storage.getDirectory(); }',
      'export function mountAcceleratorBrowser() { return navigator.serviceWorker.register("/sw.js"); }',
      'export function mountAcceleratorBrowser() { return serviceWorker.register("/sw.js"); }',
      'export function mountAcceleratorBrowser() { return window.localStorage; }',
      'export function mountAcceleratorBrowser() { return self["session" + "Storage"]; }',
      'export function mountAcceleratorBrowser() { return globalThis.indexedDB; }',
      'export function mountAcceleratorBrowser() { return window.caches; }',
      'export function mountAcceleratorBrowser() { return globalThis["CacheStorage"]; }',
      'export function mountAcceleratorBrowser() { return window.navigator.storage; }',
      'export function mountAcceleratorBrowser() { return globalThis["navigator"]["serviceWorker"]; }',
      'const scope = window; export function mountAcceleratorBrowser() { return scope.localStorage; }',
      'const nav = navigator; export function mountAcceleratorBrowser() { return nav.storage; }',
      'const nav = globalThis.navigator; export function mountAcceleratorBrowser() { return nav.serviceWorker; }',
      'const { localStorage: store } = window; export function mountAcceleratorBrowser() { return store; }',
      'const { storage } = navigator; export function mountAcceleratorBrowser() { return storage; }',
      'const { navigator: nav } = window; export function mountAcceleratorBrowser() { return nav.storage; }',
      'let scope; scope = window; export function mountAcceleratorBrowser() { return scope.localStorage; }',
      'let nav; ({ navigator: nav } = globalThis); export function mountAcceleratorBrowser() { return nav.storage; }',
      'const scope = chooseGlobal ? window : {}; const capability = getCapability(); export function mountAcceleratorBrowser() { return scope[capability]; }',
      'const capability = getCapability(); const { [capability]: selected } = navigator; export function mountAcceleratorBrowser() { return selected; }',
      'const capability = getCapability(); export function mountAcceleratorBrowser() { return window[capability]; }',
      'const scope = self; const capability = getCapability(); export function mountAcceleratorBrowser() { return scope[capability]; }',
      'const capability = getCapability(); export function mountAcceleratorBrowser() { return navigator[capability]; }',
      'const nav = window.navigator; const capability = getCapability(); export function mountAcceleratorBrowser() { return nav[capability]; }',
      `export function mountAcceleratorBrowser() {
        for (let localStorage = 0; localStorage < 1; localStorage += 1) void localStorage;
        return localStorage.getItem('key');
      }`,
      `export function mountAcceleratorBrowser(input) {
        for (const indexedDB of input) void indexedDB;
        return indexedDB.open('db');
      }`,
      `export function mountAcceleratorBrowser(input) {
        for (const caches in input) void caches;
        return caches.open('cache');
      }`,
      `export function mountAcceleratorBrowser(value) {
        switch (value) { case 0: class CacheStorage {}; return new CacheStorage(); }
        return new CacheStorage();
      }`,
      `export function mountAcceleratorBrowser(value) {
        switch (value) { case 0: const serviceWorker = {}; return serviceWorker; }
        return serviceWorker.register('/sw.js');
      }`,
      `export function mountAcceleratorBrowser(condition) {
        for (; condition;) return localStorage.getItem('key');
      }`,
      'export function mountAcceleratorBrowser() { switch (localStorage) { default: return null; } }',
    ]) expect(() => assertAllowedSource(source, 'storage-capability.ts')).toThrow(/storage|capability/i);
    const root = await mkdtemp(path.join(os.tmpdir(), 'browser-artifact-unit-'));
    roots.push(root);
    await mkdir(path.join(root, 'assets'));
    await writeFile(path.join(root, ARTIFACT_FILES[0]), markerBytes('v1'));
    await writeFile(path.join(root, ARTIFACT_FILES[1]), '.safe{}');
    const validMount = 'export const mountAcceleratorBrowser = () => { const root = document.getElementById("accelerator-browser-root"); root.textContent = "Kubikles Accelerator"; return () => root.replaceChildren(); };';
    await writeFile(path.join(root, ARTIFACT_FILES[2]), validMount);
    await expect(verifyArtifact(root, 'v1')).resolves.toMatchObject({ files: ARTIFACT_FILES });
    for (const [source, message] of [
      [`${validMount} export const extra = true;`, 'sole'],
      [`${validMount} export default mountAcceleratorBrowser;`, 'sole'],
      ['export const renamedMount = () => () => {};', 'sole'],
      ['const internalOnly = true; void internalOnly;', 'sole'],
      ['export const mountAcceleratorBrowser = 42;', 'callable'],
      [`const UpdateSecretData = true; void UpdateSecretData; ${validMount}`, 'Forbidden'],
      ['export const mountAcceleratorBrowser = () => process;', 'Node global'],
    ]) {
      await writeFile(path.join(root, ARTIFACT_FILES[0]), markerBytes('v1'));
      await writeFile(path.join(root, ARTIFACT_FILES[2]), source);
      await expect(verifyArtifact(root, 'v1')).rejects.toThrow(message);
      await expect(readFile(path.join(root, ARTIFACT_FILES[0]))).rejects.toThrow();
    }
  });

  it('waits for an asynchronous controlled-DOM mount', async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), 'browser-delayed-mount-unit-'));
    roots.push(root);
    await mkdir(path.join(root, 'assets'));
    await writeFile(path.join(root, ARTIFACT_FILES[0]), markerBytes('v1'));
    await writeFile(path.join(root, ARTIFACT_FILES[1]), '.safe{}');
    await writeFile(path.join(root, ARTIFACT_FILES[2]), `export const mountAcceleratorBrowser = () => {
      const root = document.getElementById("accelerator-browser-root");
      setTimeout(() => setImmediate(() => { root.textContent = "Kubikles Accelerator"; }), 0);
      return () => root.replaceChildren();
    };`);
    await expect(verifyArtifact(root, 'v1')).resolves.toMatchObject({ files: ARTIFACT_FILES });
  });

  it('rejects every external module dependency before controlled execution', async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), 'browser-dependency-unit-'));
    roots.push(root);
    const validMount = `export function mountAcceleratorBrowser(target, options) {
      target.textContent = String(options?.version ?? '');
      return () => target.replaceChildren();
    }`;
    const executionSentinel = path.join(root, 'import-executed');
    const dataCapability = `import { writeFileSync } from 'node:fs'; writeFileSync(${JSON.stringify(executionSentinel)}, 'executed');`;
    const dependencies = [
      `import fs from 'node:fs'; void fs; ${validMount}`,
      `import ${JSON.stringify(`data:text/javascript,${encodeURIComponent(dataCapability)}`)}; ${validMount}`,
      `import remote from 'https://example.invalid/browser.js'; void remote; ${validMount}`,
      `import React from 'react'; void React; ${validMount}`,
      `${validMount}\nfalse && import('node:fs');`,
      `${validMount}\nexport { value } from 'bare-package';`,
      `${validMount}\nexport { default } from 'data:text/javascript,export default 1';`,
      `${validMount}\nexport * from 'https://example.invalid/browser.js';`,
    ];

    for (const browserSource of dependencies) {
      await rm(root, { recursive: true, force: true });
      await mkdir(path.join(root, 'assets'), { recursive: true });
      await writeFile(path.join(root, ARTIFACT_FILES[0]), markerBytes('v1'));
      await writeFile(path.join(root, ARTIFACT_FILES[1]), '.safe{}');
      await writeFile(path.join(root, ARTIFACT_FILES[2]), browserSource);

      await expect(verifyArtifact(root, 'v1')).rejects.toThrow(/module dependenc/i);
      await expect(readFile(path.join(root, ARTIFACT_FILES[0]))).rejects.toThrow();
      await expect(readFile(executionSentinel)).rejects.toThrow();
    }
  });

  it('rejects real reachable, tree-shaken, virtual, prebundle, and transport fixtures while allowing the exact graph', async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), 'browser-graph-fixture-'));
    roots.push(root);
    const sourceRoot = path.join(root, 'src/accelerator-browser');
    await mkdir(sourceRoot, { recursive: true });
    const entry = path.join(sourceRoot, 'entry.js');
    const run = async (source, extraPlugin, extraSources = {}) => {
      await writeFile(entry, source);
      for (const [relative, code] of Object.entries(extraSources)) {
        const target = path.join(root, relative);
        await mkdir(path.dirname(target), { recursive: true });
        await writeFile(target, code);
      }
      return build({
        configFile: false,
        root,
        logLevel: 'silent',
        plugins: [extraPlugin, browserIsolationPlugin(root, 'v1')].filter(Boolean),
        build: { lib: { entry, formats: ['es'] }, outDir: path.join(root, 'dist'), emptyOutDir: true, rollupOptions: { output: { inlineDynamicImports: true } } },
      });
    };
    await expect(run("const storageLabel = 'storage'; export const mountAcceleratorBrowser = () => storageLabel;")).resolves.toBeTruthy();
    await expect(run(`
      const localStorage = { getItem: () => 'local' };
      const sessionStorage = { getItem: () => 'local' };
      const indexedDB = { open: () => 'local' };
      const caches = { open: () => 'local' };
      const navigator = { storage: {}, serviceWorker: {} };
      const serviceWorker = { register: () => 'local' };
      export function mountAcceleratorBrowser() {
        return [localStorage, sessionStorage, indexedDB, caches, navigator, serviceWorker];
      }
    `)).resolves.toBeTruthy();
    await rm(path.join(root, 'dist'), { recursive: true, force: true });
    await expect(run(`
      const capability = 'local' + 'Storage';
      export function mountAcceleratorBrowser() { return globalThis[capability]; }
    `)).rejects.toThrow(/storage|capability/i);
    await expect(readFile(path.join(root, 'dist/.kubikles-accelerator-browser-artifact'))).rejects.toThrow();
    await writeFile(path.join(root, 'forbidden.js'), 'export const hostile = true;');
    await expect(run("import { hostile } from '../../forbidden.js'; export const mountAcceleratorBrowser = () => hostile;")).rejects.toThrow('Forbidden');
    await expect(run("import { hostile } from '../../forbidden.js'; void hostile; export const mountAcceleratorBrowser = () => {};")).rejects.toThrow('Forbidden');
    for (const source of ['fetch("/x");', 'new WebSocket("wss://x");', 'new XMLHttpRequest();', 'new EventSource("/x");']) {
      await expect(run(`${source} export const mountAcceleratorBrowser = () => {};`)).rejects.toThrow('transport');
    }
    for (const [relative, source] of [
      ['src/utils/formatting.ts', 'export const unused = () => globalThis["fetch"]("/x");'],
      ['src/hooks/useNamespaceOptimization.tsx', 'export const unused = () => { const scope = self; const Socket = scope.WebSocket; return new Socket("wss://x"); };'],
    ]) {
      const imported = relative.replace(/^src\//u, '../');
      await expect(run(`import { unused } from '${imported}'; export const mountAcceleratorBrowser = () => {};`, undefined, { [relative]: source })).rejects.toThrow('transport');
    }
    const virtual = { name: 'hostile-virtual-fixture', resolveId(id) { return id === 'hostile:virtual' ? '\0hostile-virtual' : null; }, load(id) { return id === '\0hostile-virtual' ? 'export default 1;' : null; } };
    await expect(run("import hostile from 'hostile:virtual'; export const mountAcceleratorBrowser = () => hostile;", virtual)).rejects.toThrow('virtual');
    const prebundleId = path.join(root, 'node_modules/.vite/deps/hostile.js');
    const prebundle = { name: 'hostile-prebundle-fixture', resolveId(id) { return id === 'hostile:prebundle' ? prebundleId : null; }, load(id) { return id === prebundleId ? 'export default 1;' : null; } };
    await expect(run("import hostile from 'hostile:prebundle'; export const mountAcceleratorBrowser = () => hostile;", prebundle)).rejects.toThrow('prebundle');
  });
});
