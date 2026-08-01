import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { readFile, readdir, rm } from 'node:fs/promises';
import path from 'node:path';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { parse } from 'acorn';
import { base, fullAncestor, recursive } from 'acorn-walk';

export const ARTIFACT_FILES = [
  '.kubikles-browser-v1.json',
  'assets/browser.css',
  'assets/browser.js',
];

export function validateBuildVersion(value) {
  if (typeof value !== 'string' || value.length === 0 || value !== value.trim() || /\p{Cc}/u.test(value)) {
    throw new Error('BUILD_VERSION must be a nonempty literal without surrounding whitespace or control characters');
  }
  return value;
}

export function markerBytes(buildVersion) {
  validateBuildVersion(buildVersion);
  return `${JSON.stringify({ schemaVersion: 1, kind: 'kubikles-accelerator-browser', buildVersion })}\n`;
}

const normalized = value => value.split(path.sep).join('/').split('?')[0];
const allowedReactFiles = [
  '/node_modules/react/index.js',
  '/node_modules/react/jsx-runtime.js',
  '/node_modules/react/cjs/react.development.js',
  '/node_modules/react/cjs/react.production.min.js',
  '/node_modules/react/cjs/react-jsx-runtime.development.js',
  '/node_modules/react/cjs/react-jsx-runtime.production.min.js',
  '/node_modules/react-dom/client.js',
  '/node_modules/react-dom/index.js',
  '/node_modules/react-dom/cjs/react-dom.development.js',
  '/node_modules/react-dom/cjs/react-dom.production.min.js',
  '/node_modules/scheduler/index.js',
  '/node_modules/scheduler/cjs/scheduler.development.js',
  '/node_modules/scheduler/cjs/scheduler.production.min.js',
];
const isAllowedReactModule = id => allowedReactFiles.some(file => id.endsWith(file));
const sharedSourceFiles = new Set([
  'src/features/config/secrets/secretReadSourceContract.ts',
  'src/features/config/secrets/secretListOperations.ts',
  'src/hooks/useNamespaceOptimization.tsx',
  'src/utils/formatting.ts',
]);
const isFirstPartyCode = relative =>
  (/^src\/accelerator-browser\/[^?]+\.[cm]?[jt]sx?$/u.test(relative) || sharedSourceFiles.has(relative));

export function assertAllowedModuleId(id, frontendRoot) {
  const moduleId = normalized(id);
  if (moduleId.startsWith('\0')) {
    const wrapped = moduleId.slice(1);
    if (moduleId === '\0vite/preload-helper' || moduleId === '\0commonjsHelpers.js' ||
        isAllowedReactModule(wrapped)) return;
    throw new Error(`Forbidden Accelerator Browser virtual module: ${moduleId.slice(1)}`);
  }
  if (moduleId.includes('/node_modules/.vite/')) {
    if (/\/node_modules\/\.vite\/deps\/(react|react-dom_client|scheduler)(\.js)?$/u.test(moduleId)) return;
    throw new Error(`Forbidden Accelerator Browser prebundle module: ${moduleId}`);
  }
  const relative = normalized(path.relative(frontendRoot, moduleId));
  const allowedSource = isFirstPartyCode(relative) || relative === 'src/accelerator-browser/browser.css';
  const allowedDependency = isAllowedReactModule(moduleId);
  if (!allowedSource && !allowedDependency) throw new Error(`Forbidden Accelerator Browser module: ${relative}`);
}

const transportGlobals = new Set(['fetch', 'WebSocket', 'XMLHttpRequest', 'EventSource']);
const nodeGlobals = new Set(['process', 'Buffer', 'require', 'module', 'global']);
const browserGlobals = new Set(['globalThis', 'window', 'self']);
const storageGlobals = new Set(['localStorage', 'sessionStorage', 'indexedDB', 'caches', 'CacheStorage', 'serviceWorker']);
const navigatorStorageProperties = new Set(['storage', 'serviceWorker']);
const execFileAsync = promisify(execFile);
const frontendRootForVerifier = path.dirname(path.dirname(fileURLToPath(import.meta.url)));

const bindPattern = (pattern, bind) => {
  if (!pattern) return;
  if (pattern.type === 'Identifier') bind(pattern.name);
  else if (pattern.type === 'RestElement') bindPattern(pattern.argument, bind);
  else if (pattern.type === 'AssignmentPattern') bindPattern(pattern.left, bind);
  else if (pattern.type === 'ArrayPattern') pattern.elements.forEach(value => bindPattern(value, bind));
  else if (pattern.type === 'ObjectPattern') pattern.properties.forEach(property =>
    bindPattern(property.type === 'RestElement' ? property.argument : property.value, bind));
};

function analyzeGlobalReferences(source, id) {
  let ast;
  try {
    ast = parse(source, { ecmaVersion: 'latest', sourceType: 'module' });
  } catch (error) {
    throw new Error(`Unable to parse Accelerator Browser code in ${id}`, { cause: error });
  }

  const scopeNodes = new WeakMap();
  const declarators = [];
  const assignments = [];
  const makeScope = (parent, variable = false) => ({ parent, variable, bindings: new Map() });
  const rootScope = makeScope(null, true);
  const bind = (scope, name) => {
    if (!scope.bindings.has(name)) scope.bindings.set(name, {});
    return scope.bindings.get(name);
  };
  const variableScope = scope => {
    let current = scope;
    while (!current.variable) current = current.parent;
    return current;
  };
  const functionVisitor = (node, scope, callback, declaration) => {
    if (declaration && node.id) bind(scope, node.id.name);
    const functionScope = makeScope(scope, true);
    scopeNodes.set(node, functionScope);
    if (!declaration && node.id) bind(functionScope, node.id.name);
    node.params.forEach(parameter => bindPattern(parameter, name => bind(functionScope, name)));
    node.params.forEach(parameter => callback(parameter, functionScope));
    callback(node.body, functionScope);
  };
  recursive(ast, rootScope, {
    Program(node, scope, callback) {
      scopeNodes.set(node, scope);
      node.body.forEach(child => callback(child, scope));
    },
    BlockStatement(node, scope, callback) {
      const blockScope = makeScope(scope);
      scopeNodes.set(node, blockScope);
      node.body.forEach(child => callback(child, blockScope));
    },
    FunctionDeclaration(node, scope, callback) { functionVisitor(node, scope, callback, true); },
    FunctionExpression(node, scope, callback) { functionVisitor(node, scope, callback, false); },
    ArrowFunctionExpression(node, scope, callback) { functionVisitor(node, scope, callback, false); },
    VariableDeclaration(node, scope, callback) {
      const declarationScope = node.kind === 'var' ? variableScope(scope) : scope;
      for (const declaration of node.declarations) {
        bindPattern(declaration.id, name => bind(declarationScope, name));
        declarators.push({ pattern: declaration.id, init: declaration.init, bindingScope: declarationScope, referenceScope: scope });
        if (declaration.init) callback(declaration.init, scope);
      }
    },
    AssignmentExpression(node, scope, callback) {
      assignments.push({ pattern: node.left, init: node.right, bindingScope: scope, referenceScope: scope });
      callback(node.left, scope);
      callback(node.right, scope);
    },
    ForStatement(node, scope, callback) {
      const loopScope = makeScope(scope);
      scopeNodes.set(node, loopScope);
      if (node.init) callback(node.init, loopScope);
      if (node.test) callback(node.test, loopScope);
      if (node.update) callback(node.update, loopScope);
      callback(node.body, loopScope);
    },
    ForInStatement(node, scope, callback) {
      const loopScope = makeScope(scope);
      scopeNodes.set(node, loopScope);
      callback(node.left, loopScope);
      callback(node.right, loopScope);
      callback(node.body, loopScope);
    },
    ForOfStatement(node, scope, callback) {
      const loopScope = makeScope(scope);
      scopeNodes.set(node, loopScope);
      callback(node.left, loopScope);
      callback(node.right, loopScope);
      callback(node.body, loopScope);
    },
    SwitchStatement(node, scope, callback) {
      callback(node.discriminant, scope);
      const switchScope = makeScope(scope);
      for (const switchCase of node.cases) {
        scopeNodes.set(switchCase, switchScope);
        if (switchCase.test) callback(switchCase.test, switchScope);
        switchCase.consequent.forEach(statement => callback(statement, switchScope));
      }
    },
    ClassDeclaration(node, scope, callback) {
      if (node.id) bind(scope, node.id.name);
      if (node.superClass) callback(node.superClass, scope);
      callback(node.body, scope);
    },
    ClassExpression(node, scope, callback) {
      const classScope = makeScope(scope);
      scopeNodes.set(node, classScope);
      if (node.id) bind(classScope, node.id.name);
      if (node.superClass) callback(node.superClass, scope);
      callback(node.body, classScope);
    },
    ImportDeclaration(node, scope) {
      node.specifiers.forEach(specifier => bind(scope, specifier.local.name));
    },
    CatchClause(node, scope, callback) {
      const catchScope = makeScope(scope);
      scopeNodes.set(node, catchScope);
      bindPattern(node.param, name => bind(catchScope, name));
      callback(node.body, catchScope);
    },
  }, base);

  const scopeFor = ancestors => {
    for (let index = ancestors.length - 1; index >= 0; index -= 1) {
      const scope = scopeNodes.get(ancestors[index]);
      if (scope) return scope;
    }
    return rootScope;
  };
  const resolve = (scope, name) => {
    for (let current = scope; current; current = current.parent) {
      if (current.bindings.has(name)) return current.bindings.get(name);
    }
    return null;
  };
  const inRange = (node, range) => range && node.start >= range.start && node.end <= range.end;
  const isDeclaration = (node, ancestors) => ancestors.some(ancestor =>
    (ancestor.type === 'VariableDeclarator' && inRange(node, ancestor.id)) ||
    ((ancestor.type === 'FunctionDeclaration' || ancestor.type === 'FunctionExpression' || ancestor.type === 'ArrowFunctionExpression') &&
      (inRange(node, ancestor.id) || ancestor.params.some(parameter => inRange(node, parameter)))) ||
    ((ancestor.type === 'ClassDeclaration' || ancestor.type === 'ClassExpression') && inRange(node, ancestor.id)) ||
    (ancestor.type === 'CatchClause' && inRange(node, ancestor.param)) ||
    (ancestor.type.startsWith('Import') && ancestor.type.endsWith('Specifier')));
  const isReference = (node, ancestors) => {
    if (isDeclaration(node, ancestors)) return false;
    const parent = ancestors[ancestors.length - 2];
    if (!parent) return true;
    if ((parent.type === 'MemberExpression' || parent.type === 'PropertyDefinition' || parent.type === 'MethodDefinition') &&
        parent.property === node && !parent.computed) return false;
    if (parent.type === 'Property' && parent.key === node && !parent.computed && !parent.shorthand) return false;
    if ((parent.type === 'LabeledStatement' || parent.type === 'BreakStatement' || parent.type === 'ContinueStatement') && parent.label === node) return false;
    if (parent.type === 'ExportSpecifier' && parent.exported === node) return false;
    return true;
  };
  const staticProperty = node => {
    if (!node) return null;
    if (node.type === 'Literal' && node.value !== undefined && typeof node.value !== 'object') return String(node.value);
    if (node.type === 'TemplateLiteral' && node.expressions.length === 0) return node.quasis[0].value.cooked;
    if (node.type === 'BinaryExpression' && node.operator === '+') {
      const left = staticProperty(node.left);
      const right = staticProperty(node.right);
      return left === null || right === null ? null : left + right;
    }
    return null;
  };
  const propertyName = node => node.computed ? staticProperty(node.property) : node.property.type === 'Identifier' ? node.property.name : null;
  const globalAliases = new Set();
  const navigatorAliases = new Set();
  const GLOBAL_OBJECT = 1;
  const NAVIGATOR_OBJECT = 2;
  const objectKinds = (expression, scope) => {
    if (!expression) return 0;
    if (expression.type === 'ChainExpression') return objectKinds(expression.expression, scope);
    if (expression.type === 'Identifier') {
      const binding = resolve(scope, expression.name);
      if (binding) return (globalAliases.has(binding) ? GLOBAL_OBJECT : 0) | (navigatorAliases.has(binding) ? NAVIGATOR_OBJECT : 0);
      if (browserGlobals.has(expression.name)) return GLOBAL_OBJECT;
      if (expression.name === 'navigator') return NAVIGATOR_OBJECT;
      return 0;
    }
    if (expression.type === 'MemberExpression') {
      const parentKinds = objectKinds(expression.object, scope);
      const name = propertyName(expression);
      let kinds = 0;
      if ((parentKinds & GLOBAL_OBJECT) && browserGlobals.has(name)) kinds |= GLOBAL_OBJECT;
      if ((parentKinds & GLOBAL_OBJECT) && name === 'navigator') kinds |= NAVIGATOR_OBJECT;
      return kinds;
    }
    if (expression.type === 'ConditionalExpression') {
      return objectKinds(expression.consequent, scope) | objectKinds(expression.alternate, scope);
    }
    if (expression.type === 'LogicalExpression') {
      return objectKinds(expression.left, scope) | objectKinds(expression.right, scope);
    }
    if (expression.type === 'SequenceExpression') return objectKinds(expression.expressions.at(-1), scope);
    if (expression.type === 'AssignmentExpression') return objectKinds(expression.right, scope);
    return 0;
  };
  const addPatternAliases = (pattern, kinds, scope) => {
    if (!pattern || kinds === 0) return false;
    if (pattern.type === 'AssignmentPattern') return addPatternAliases(pattern.left, kinds, scope);
    if (pattern.type === 'Identifier') {
      const binding = resolve(scope, pattern.name);
      if (!binding) return false;
      let added = false;
      if ((kinds & GLOBAL_OBJECT) && !globalAliases.has(binding)) {
        globalAliases.add(binding);
        added = true;
      }
      if ((kinds & NAVIGATOR_OBJECT) && !navigatorAliases.has(binding)) {
        navigatorAliases.add(binding);
        added = true;
      }
      return added;
    }
    if (pattern.type !== 'ObjectPattern') return false;
    let added = false;
    for (const property of pattern.properties) {
      if (property.type !== 'Property') continue;
      const name = property.computed ? staticProperty(property.key) : property.key.name ?? String(property.key.value);
      let propertyKinds = 0;
      if ((kinds & GLOBAL_OBJECT) && browserGlobals.has(name)) propertyKinds |= GLOBAL_OBJECT;
      if ((kinds & GLOBAL_OBJECT) && name === 'navigator') propertyKinds |= NAVIGATOR_OBJECT;
      if (addPatternAliases(property.value, propertyKinds, scope)) added = true;
    }
    return added;
  };
  const aliasSources = [...declarators, ...assignments];
  let changed = true;
  while (changed) {
    changed = false;
    for (const declaration of aliasSources) {
      const kinds = objectKinds(declaration.init, declaration.referenceScope);
      if (addPatternAliases(declaration.pattern, kinds, declaration.bindingScope)) changed = true;
    }
  }

  const storageError = name => new Error(`Forbidden Accelerator Browser storage capability in ${id}: ${name}`);
  const inspectObjectPattern = (pattern, kinds) => {
    if (pattern.type === 'AssignmentPattern') return inspectObjectPattern(pattern.left, kinds);
    if (pattern.type !== 'ObjectPattern') return;
    for (const property of pattern.properties) {
      if (property.type === 'RestElement') throw storageError('dynamic global destructuring');
      const name = property.computed ? staticProperty(property.key) : property.key.name ?? String(property.key.value);
      if (name === null) throw storageError('dynamic computed property');
      if (kinds & GLOBAL_OBJECT) {
        if (transportGlobals.has(name)) throw new Error(`Forbidden Accelerator Browser transport in ${id}: ${name}`);
        if (nodeGlobals.has(name)) throw new Error(`Forbidden Accelerator Browser Node global in ${id}: ${name}`);
        if (storageGlobals.has(name)) throw storageError(name);
      }
      if ((kinds & NAVIGATOR_OBJECT) && navigatorStorageProperties.has(name)) throw storageError(`navigator.${name}`);
      let propertyKinds = 0;
      if ((kinds & GLOBAL_OBJECT) && browserGlobals.has(name)) propertyKinds |= GLOBAL_OBJECT;
      if ((kinds & GLOBAL_OBJECT) && name === 'navigator') propertyKinds |= NAVIGATOR_OBJECT;
      if (propertyKinds) inspectObjectPattern(property.value, propertyKinds);
    }
  };
  for (const declaration of aliasSources) {
    const kinds = objectKinds(declaration.init, declaration.referenceScope);
    if (kinds) inspectObjectPattern(declaration.pattern, kinds);
  }

  fullAncestor(ast, (node, _state, ancestors) => {
    const scope = scopeFor(ancestors);
    if (node.type === 'Identifier' && isReference(node, ancestors) && !resolve(scope, node.name)) {
      if (transportGlobals.has(node.name)) throw new Error(`Forbidden Accelerator Browser transport in ${id}: ${node.name}`);
      if (nodeGlobals.has(node.name)) throw new Error(`Forbidden Accelerator Browser Node global in ${id}: ${node.name}`);
      if (storageGlobals.has(node.name)) throw storageError(node.name);
    }
    if (node.type === 'MemberExpression') {
      const kinds = objectKinds(node.object, scope);
      if (!kinds) return;
      const name = propertyName(node);
      if (name === null) throw storageError('dynamic computed property');
      if (kinds & GLOBAL_OBJECT) {
        if (transportGlobals.has(name)) throw new Error(`Forbidden Accelerator Browser transport in ${id}: ${name}`);
        if (nodeGlobals.has(name)) throw new Error(`Forbidden Accelerator Browser Node global in ${id}: ${name}`);
        if (storageGlobals.has(name)) throw storageError(name);
      }
      if ((kinds & NAVIGATOR_OBJECT) && navigatorStorageProperties.has(name)) throw storageError(`navigator.${name}`);
    }
  });
  return ast;
}

export function assertNoIndependentTransport(source, id = 'source') {
  analyzeGlobalReferences(source, id);
}

export function assertAllowedSource(source, id = 'source') {
  analyzeGlobalReferences(source, id);
  for (const token of forbiddenOutput) {
    if (source.includes(token)) throw new Error(`Forbidden Accelerator Browser source token in ${id}: ${token}`);
  }
}

export function browserIsolationPlugin(frontendRoot, buildVersion) {
  return {
    name: 'kubikles-accelerator-browser-isolation',
    moduleParsed(info) {
      assertAllowedModuleId(info.id, frontendRoot);
      for (const id of [...info.importedIds, ...info.dynamicallyImportedIds]) assertAllowedModuleId(id, frontendRoot);
    },
    transform(code, id) {
      const relative = normalized(path.relative(frontendRoot, normalized(id)));
      if (isFirstPartyCode(relative)) assertAllowedSource(code, relative);
      return null;
    },
    generateBundle(_options, bundle) {
      for (const id of this.getModuleIds()) assertAllowedModuleId(id, frontendRoot);
      for (const item of Object.values(bundle)) {
        if (item.type === 'chunk') {
          for (const id of Object.keys(item.modules)) assertAllowedModuleId(id, frontendRoot);
        }
      }
      this.emitFile({ type: 'asset', fileName: '.kubikles-browser-v1.json', source: markerBytes(buildVersion) });
    },
  };
}

const forbiddenOutput = [
  'wailsjs', 'AgentRouter',
  'UpdateSecretYaml', 'UpdateSecretData', 'DeleteSecret', 'SaveDataEntryValue', 'GetAllCertificateInfo',
  'monaco-editor', 'xterm', '@xyflow', 'ResourceList', 'SecretEditor', 'ApplyYAML', 'ListNamespaces',
  'navigator.clipboard', 'writeText(', 'createObjectURL', 'showSaveFilePicker', 'window.open', 'location.reload', 'history.pushState',
];

async function walk(root, relative = '') {
  const entries = await readdir(path.join(root, relative), { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const next = path.posix.join(relative, entry.name);
    if (entry.isDirectory()) files.push(...await walk(root, next));
    else files.push(next);
  }
  return files.sort();
}

function assertSoleMountExport(ast) {
  const exports = [];
  const collectPattern = pattern => {
    if (pattern.type === 'Identifier') exports.push(pattern.name);
    else if (pattern.type === 'RestElement') collectPattern(pattern.argument);
    else if (pattern.type === 'AssignmentPattern') collectPattern(pattern.left);
    else if (pattern.type === 'ArrayPattern') pattern.elements.filter(Boolean).forEach(collectPattern);
    else if (pattern.type === 'ObjectPattern') pattern.properties.forEach(property =>
      collectPattern(property.type === 'RestElement' ? property.argument : property.value));
  };
  for (const statement of ast.body) {
    if (statement.type === 'ExportDefaultDeclaration' || statement.type === 'ExportAllDeclaration') exports.push('default');
    if (statement.type !== 'ExportNamedDeclaration') continue;
    for (const specifier of statement.specifiers) exports.push(specifier.exported.name ?? specifier.exported.value);
    const declaration = statement.declaration;
    if (declaration?.type === 'VariableDeclaration') declaration.declarations.forEach(item => collectPattern(item.id));
    else if ((declaration?.type === 'FunctionDeclaration' || declaration?.type === 'ClassDeclaration') && declaration.id) exports.push(declaration.id.name);
  }
  if (exports.length !== 1 || exports[0] !== 'mountAcceleratorBrowser') {
    throw new Error('Accelerator Browser entry must have the sole named export mountAcceleratorBrowser');
  }
}

function assertNoModuleDependencies(ast, id) {
  fullAncestor(ast, node => {
    const isImport = node.type === 'ImportDeclaration' || node.type === 'ImportExpression';
    const isReExport = node.type.startsWith('Export') && node.source;
    if (isImport || isReExport) {
      throw new Error(`Accelerator Browser entry must not have module dependencies in ${id}: ${node.type}`);
    }
  });
}

async function verifyArtifactContents(root, buildVersion) {
  validateBuildVersion(buildVersion);
  const files = await walk(root);
  if (JSON.stringify(files) !== JSON.stringify(ARTIFACT_FILES)) throw new Error(`Unexpected Accelerator Browser files: ${files.join(', ')}`);
  const javascript = await readFile(path.join(root, 'assets/browser.js'), 'utf8');
  const css = await readFile(path.join(root, 'assets/browser.css'), 'utf8');
  const ast = analyzeGlobalReferences(javascript, 'assets/browser.js');
  assertNoModuleDependencies(ast, 'assets/browser.js');
  assertSoleMountExport(ast);
  for (const token of forbiddenOutput) {
    if (javascript.includes(token) || css.includes(token)) throw new Error(`Forbidden Accelerator Browser output token: ${token}`);
  }
  if (javascript.includes('sourceMappingURL')) throw new Error('Unexpected Accelerator Browser source map reference');
  await verifyControlledDomMount(root);
  const marker = await readFile(path.join(root, ARTIFACT_FILES[0]), 'utf8');
  if (marker !== markerBytes(buildVersion)) throw new Error('Accelerator Browser marker mismatch');
  const hashes = {};
  for (const file of files) {
    const bytes = await readFile(path.join(root, file));
    hashes[file] = createHash('sha256').update(bytes).digest('hex');
  }
  const aggregate = createHash('sha256').update(files.map(file => `${file}\0${hashes[file]}\n`).join('')).digest('hex');
  return { files, hashes, aggregate };
}

export async function verifyArtifact(root, buildVersion) {
  try {
    return await verifyArtifactContents(root, buildVersion);
  } catch (error) {
    await rm(path.join(root, ARTIFACT_FILES[0]), { force: true });
    throw error;
  }
}

export async function verifyControlledDomMount(root) {
  const script = String.raw`
    import path from 'node:path';
    import { pathToFileURL } from 'node:url';
    import { JSDOM } from 'jsdom';
    const artifactRoot = process.argv[1];
    const dom = new JSDOM('<div id="accelerator-browser-root"></div>', { url: 'https://example.test/accelerator/browser/' });
    const controlled = new Map();
    const replaceGlobal = (name, value) => {
      if (!controlled.has(name)) controlled.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
      if (!Reflect.deleteProperty(globalThis, name)) throw new Error('Unable to isolate controlled DOM global: ' + name);
      if (value !== undefined) Object.defineProperty(globalThis, name, { value, writable: true, configurable: true });
    };
    const restoreGlobals = names => {
      for (const name of names) {
        if (!controlled.has(name)) continue;
        const descriptor = controlled.get(name);
        Reflect.deleteProperty(globalThis, name);
        if (descriptor) Object.defineProperty(globalThis, name, descriptor);
        controlled.delete(name);
      }
    };
    const waitFor = async (predicate, message) => {
      const deadline = Date.now() + 1000;
      while (!predicate()) {
        if (Date.now() >= deadline) throw new Error(message);
        await new Promise(resolve => setImmediate(resolve));
      }
    };
    const nodeGlobalNames = ['process', 'Buffer', 'require', 'module', 'global'];
    try {
      replaceGlobal('window', dom.window);
      replaceGlobal('document', dom.window.document);
      replaceGlobal('navigator', dom.window.navigator);
      for (const name of nodeGlobalNames) replaceGlobal(name, undefined);
      let cleanup;
      try {
        const url = pathToFileURL(path.resolve(artifactRoot, 'assets/browser.js'));
        url.searchParams.set('controlled', 'process-free');
        const browserModule = await import(url.href);
        if (Object.keys(browserModule).join('\0') !== 'mountAcceleratorBrowser' || typeof browserModule.mountAcceleratorBrowser !== 'function') {
          throw new Error('Accelerator Browser controlled-DOM export is not the sole callable mount');
        }
        cleanup = browserModule.mountAcceleratorBrowser({
          ListSecretsMetadata: async () => [],
          GetSecretData: async () => [],
          GetSecretYaml: async () => '',
          CancelListRequest: async () => undefined,
          SubscribeSecretWatcher: async () => ({ watcherSpecId: 'controlled-spec' }),
          UnsubscribeSecretWatcher: async () => undefined,
          close: async () => undefined,
          events: { subscribe: () => () => {} },
        });
        if (typeof cleanup !== 'function') throw new Error('Accelerator Browser mount did not return cleanup');
      } finally {
        restoreGlobals(nodeGlobalNames);
      }
      await waitFor(
        () => dom.window.document.body.textContent.includes('Kubikles Accelerator'),
        'Accelerator Browser controlled-DOM mount failed',
      );
      cleanup();
      await waitFor(
        () => !dom.window.document.getElementById('accelerator-browser-root')?.childNodes.length,
        'Accelerator Browser controlled-DOM cleanup failed',
      );
    } finally {
      restoreGlobals([...controlled.keys()]);
      dom.window.close();
    }
  `;
  await execFileAsync(process.execPath, ['--input-type=module', '--eval', script, path.resolve(root)], {
    cwd: frontendRootForVerifier,
    maxBuffer: 1024 * 1024,
  });
}
