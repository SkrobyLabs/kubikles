import test from 'node:test';
import assert from 'node:assert/strict';
import { ticketFromFragment, clearFragment, start } from './bootstrap.js';

test('fragment ticket parser accepts only one canonical RawURL ticket', () => {
  const ticket = 'A'.repeat(43);
  assert.equal(ticketFromFragment(`#ticket=${ticket}`), ticket);
  for (const value of ['', '#ticket=', '#ticket=short', `#ticket=${ticket}&x=1`, `#ticket=${ticket}%3D`, '#other=x']) assert.equal(ticketFromFragment(value), null);
});

test('fragment clear is synchronous and fixed', () => {
  const calls = [];
  clearFragment({ replaceState: (...args) => calls.push(args) });
  assert.deepEqual(calls, [[null, '', '/accelerator/browser/']]);
});

test('bootstrap accepts only the negotiated connected envelope and flattens allowed events', async () => {
  const listeners = []; let socket;
  class Socket {
    constructor() { socket = this; this.protocol = 'kubikles-accelerator-v1'; queueMicrotask(() => this.onmessage({ data: JSON.stringify({ type: 'event', name: 'connected', data: { instanceId: 'instance', sessionId: 'session', generation: 1, resumed: false } }) })); }
    close() { this.closed = true; }
  }
  const root = { textContent: '' };
  const state = await start({
    location: { hash: `#ticket=${'A'.repeat(43)}`, origin: 'http://localhost' }, history: { replaceState() {} }, document: { getElementById: () => root }, WebSocket: Socket,
    fetch: async url => ({ ok: true, json: async () => url.includes('session') ? { bearer: 'B'.repeat(43), expiresAt: 'x' } : { runtime: 'accelerator', build: {}, instanceId: 'instance', capabilities: [], capabilityDiagnostics: [] } }),
    importModule: async () => ({ mountAcceleratorBrowser: facade => { facade.events.subscribe(event => listeners.push(event)); return () => {}; } }),
  });
  socket.onmessage({ data: JSON.stringify({ type: 'event', name: 'resource-event', data: { safe: true } }) });
  assert.deepEqual(listeners, [{ type: 'resource-event', data: { safe: true } }]);
  state.socket.onmessage({ data: JSON.stringify({ type: 'event', name: 'forged', data: {} }) });
  assert.equal(root.textContent, 'Reopen from Kubikles');
  assert.equal(socket.closed, true);
});

test('bootstrap rejects malformed first frames and handshake close', async () => {
  for (const first of [JSON.stringify({ type: 'connected', data: {} }), JSON.stringify({ type: 'event', name: 'connected', data: { instanceId: 'wrong', sessionId: 's', generation: 1, resumed: false } })]) {
    let socket; const root = { textContent: '' };
    class Socket { constructor() { socket = this; this.protocol = 'kubikles-accelerator-v1'; queueMicrotask(() => this.onmessage({ data: first })); } close() { this.closed = true; } }
    await start({ location: { hash: `#ticket=${'A'.repeat(43)}`, origin: 'http://localhost' }, history: { replaceState() {} }, document: { getElementById: () => root }, WebSocket: Socket, fetch: async url => ({ ok: true, json: async () => url.includes('session') ? { bearer: 'B'.repeat(43), expiresAt: 'x' } : { runtime: 'accelerator', build: {}, instanceId: 'instance', capabilities: [], capabilityDiagnostics: [] } }), importModule: async () => { throw Error('must not import'); } });
    assert.equal(root.textContent, 'Reopen from Kubikles'); assert.equal(socket.closed, true);
  }
});

test('terminal state survives import and mount races exactly once', async () => {
  let socket; let resolveImport; let cleanups = 0; const terminalEvents = [];
  class Socket {
    constructor() { socket = this; this.protocol = 'kubikles-accelerator-v1'; queueMicrotask(() => this.onmessage({ data: JSON.stringify({ type: 'event', name: 'connected', data: { instanceId: 'instance', sessionId: 'session', generation: 1, resumed: false } }) })); }
    close() { this.closed = (this.closed || 0) + 1; }
  }
  const root = { textContent: '' };
  const pending = start({
    location: { hash: `#ticket=${'A'.repeat(43)}`, origin: 'http://localhost' }, history: { replaceState() {} }, document: { getElementById: () => root }, WebSocket: Socket,
    fetch: async url => ({ ok: true, json: async () => url.includes('session') ? { bearer: 'B'.repeat(43), expiresAt: 'x' } : { runtime: 'accelerator', build: {}, instanceId: 'instance', capabilities: [], capabilityDiagnostics: [] } }),
    importModule: () => new Promise(resolve => { resolveImport = resolve; }),
  });
  await new Promise(resolve => setImmediate(resolve));
  const importClose = socket.onclose; const importError = socket.onerror; importClose(); importError();
  resolveImport({ mountAcceleratorBrowser: facade => { facade.events.subscribe(event => terminalEvents.push(event)); cleanups++; return () => { cleanups++; }; } });
  const state = await pending;
  assert.equal(state.bearer, null); assert.equal(root.textContent, 'Reopen from Kubikles'); assert.equal(socket.closed, 1); assert.equal(cleanups, 0);

  // Once mounted, close/error terminalize once, clean up once, and publish the
  // fixed terminal envelope rather than an application event.
  let mountedSocket; let mountedCleanup = 0; const events = [];
  class MountedSocket extends Socket { constructor() { super(); mountedSocket = this; } }
  const mounted = await start({
    location: { hash: `#ticket=${'A'.repeat(43)}`, origin: 'http://localhost' }, history: { replaceState() {} }, document: { getElementById: () => root }, WebSocket: MountedSocket,
    fetch: async url => ({ ok: true, json: async () => url.includes('session') ? { bearer: 'B'.repeat(43), expiresAt: 'x' } : { runtime: 'accelerator', build: {}, instanceId: 'instance', capabilities: [], capabilityDiagnostics: [] } }),
    importModule: async () => ({ mountAcceleratorBrowser: facade => { facade.events.subscribe(event => events.push(event)); return () => { mountedCleanup++; }; } }),
  });
  const mountedClose = mountedSocket.onclose; const mountedError = mountedSocket.onerror; mountedClose(); mountedError();
  assert.equal(mounted.closed, true); assert.equal(mounted.bearer, null); assert.equal(mountedCleanup, 1); assert.deepEqual(events, [{ type: 'terminal' }]);
});

test('rejected fetch and JSON terminalize before any mount', async () => {
  for (const response of [Promise.reject(Error('network')), { ok: true, json: async () => { throw Error('json'); } }]) {
    let mounted = false; const root = { textContent: '' };
    const state = await start({ location: { hash: `#ticket=${'A'.repeat(43)}`, origin: 'http://localhost' }, history: { replaceState() {} }, document: { getElementById: () => root }, WebSocket: class {}, fetch: async () => response, importModule: async () => { mounted = true; return {}; } });
    assert.equal(state.closed, true); assert.equal(state.bearer, null); assert.equal(mounted, false); assert.equal(root.textContent, 'Reopen from Kubikles');
  }
});

test('post-mount RPC failures publish terminal before cleanup and finish teardown exactly once', async t => {
  const failures = {
    'rejected fetch': async () => { throw Error('network detail'); },
    'rejected JSON': async () => ({ ok: true, json: async () => { throw Error('json detail'); } }),
    'non-2xx response': async () => ({ ok: false, json: async () => ({ data: 'must not decode' }) }),
    'malformed envelope': async () => ({ ok: true, json: async () => ({ result: 'not data' }) }),
  };
  for (const [name, rpc] of Object.entries(failures)) {
    await t.test(name, async () => {
      const order = []; let socket; let facade; let uiWrites = 0; let closeCalls = 0;
      const root = {
        get textContent() { return ''; },
        set textContent(value) { uiWrites++; order.push(`ui:${value}`); },
      };
      class Socket {
        constructor() {
          socket = this;
          this.protocol = 'kubikles-accelerator-v1';
          queueMicrotask(() => this.onmessage({ data: JSON.stringify({ type: 'event', name: 'connected', data: { instanceId: 'instance', sessionId: 'session', generation: 1, resumed: false } }) }));
        }
        close() { closeCalls++; order.push('socket-close'); }
      }
      let fetchCalls = 0;
      const state = await start({
        location: { hash: `#ticket=${'A'.repeat(43)}`, origin: 'http://localhost' },
        history: { replaceState() {} },
        document: { getElementById: () => root },
        WebSocket: Socket,
        fetch: async url => {
          fetchCalls++;
          if (url.includes('session')) return { ok: true, json: async () => ({ bearer: 'B'.repeat(43), expiresAt: 'x' }) };
          if (url.includes('info')) return { ok: true, json: async () => ({ runtime: 'accelerator', build: {}, instanceId: 'instance', capabilities: [], capabilityDiagnostics: [] }) };
          return rpc();
        },
        importModule: async () => ({
          mountAcceleratorBrowser: value => {
            facade = value;
            value.events.subscribe(event => { order.push(`throwing-listener:${event.type}`); throw Error('listener detail'); });
            value.events.subscribe(event => order.push(`listener:${event.type}`));
            return () => { order.push('cleanup'); throw Error('cleanup detail'); };
          },
        }),
      });
      const postMountMessage = socket.onmessage;
      const postMountClose = socket.onclose;
      const postMountError = socket.onerror;
      await assert.rejects(facade.ListSecretsMetadata('request', '', false));
      assert.equal(fetchCalls, 3);
      assert.equal(state.closed, true);
      assert.equal(state.bearer, null);
      assert.deepEqual(order, [
        'throwing-listener:terminal',
        'listener:terminal',
        'cleanup',
        'socket-close',
        'ui:Reopen from Kubikles',
      ]);
      assert.equal(closeCalls, 1);
      assert.equal(uiWrites, 1);

      postMountMessage({ data: JSON.stringify({ type: 'event', name: 'resource-event', data: { late: true } }) });
      postMountClose();
      postMountError();
      await assert.rejects(facade.GetSecretData('namespace', 'name'));
      assert.equal(fetchCalls, 3, 'terminal facade must make no later request');
      assert.equal(closeCalls, 1);
      assert.equal(uiWrites, 1);
      assert.deepEqual(order, [
        'throwing-listener:terminal',
        'listener:terminal',
        'cleanup',
        'socket-close',
        'ui:Reopen from Kubikles',
      ]);
    });
  }
});
