const terminalText = "Reopen from Kubikles";
const protocol = "kubikles-accelerator-v1";
const eventNames = new Set(["resource-event", "watcher-status", "watcher-error", "list-progress"]);
export function ticketFromFragment(fragment) { const m = /^#ticket=([A-Za-z0-9_-]{43})$/.exec(fragment); return m ? m[1] : null; }
export function clearFragment(history) { history.replaceState(null, "", "/accelerator/browser/"); }
function exactObject(value, keys) { return value && typeof value === "object" && !Array.isArray(value) && Object.keys(value).sort().join("\0") === keys.slice().sort().join("\0"); }
function terminal(document, state) {
 if (state.closed) return;
 state.closed = true;
 for (const listener of [...state.listeners]) { try { listener({type:"terminal"}); } catch {} }
 state.listeners.clear();
 if (state.cleanup) { const cleanup = state.cleanup; state.cleanup = null; try { cleanup(); } catch {} }
 if (state.socket) {
  const socket = state.socket;
  socket.onopen = null; socket.onmessage = null; socket.onerror = null; socket.onclose = null;
  try { socket.close(); } catch {}
 }
 state.bearer = null;
 try { const root = document.getElementById("accelerator-browser-root"); if (root) root.textContent = terminalText; } catch {}
}
function connected(frame, instanceId) {
 const data = frame && frame.data;
 return exactObject(frame,["type","name","data"]) && frame.type === "event" && frame.name === "connected" &&
  exactObject(data,["instanceId","sessionId","generation","resumed"]) && data.instanceId === instanceId &&
  typeof data.sessionId === "string" && data.sessionId !== "" && data.generation === 1 && data.resumed === false;
}
export async function start(deps) {
 const state = { bearer: null, socket: null, cleanup: null, closed: false, listeners: new Set() };
 const ticket = ticketFromFragment(deps.location.hash); clearFragment(deps.history); if (!ticket) { terminal(deps.document, state); return state; }
 try {
  const exchange = await deps.fetch("/api/accelerator-browser-session", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({ticket}),credentials:"omit",cache:"no-store",redirect:"error",referrerPolicy:"no-referrer"});
  if (state.closed || !exchange.ok) throw Error(); const exchanged = await exchange.json();
  if (state.closed || !exactObject(exchanged,["bearer","expiresAt"]) || !/^[A-Za-z0-9_-]{43}$/.test(exchanged.bearer)) throw Error(); state.bearer = exchanged.bearer;
  const infoResponse = await deps.fetch("/api/accelerator-info", {headers:{Authorization:"Bearer "+state.bearer},credentials:"omit",cache:"no-store",redirect:"error",referrerPolicy:"no-referrer"});
  if (state.closed || !infoResponse.ok) throw Error(); const info = await infoResponse.json();
  if (state.closed || !exactObject(info,["runtime","build","instanceId","capabilities","capabilityDiagnostics"]) || typeof info.instanceId !== "string" || !info.instanceId) throw Error();
  const socket = new deps.WebSocket(deps.location.origin.replace(/^http/, "ws")+"/ws", [protocol, "kubikles-accelerator-browser-bearer."+state.bearer]); state.socket = socket;
  await new Promise((resolve,reject) => {
   // Keep terminal handling installed while the asynchronous bootstrap is in
   // flight.  A close must never merely reject a promise and then allow a
   // later import or mount to continue.
   const fail = () => { terminal(deps.document, state); reject(Error()); };
   socket.onclose = fail; socket.onerror = fail;
   socket.onmessage = event => { try { if (!connected(JSON.parse(event.data), info.instanceId)) throw Error(); resolve(); } catch { fail(); } };
  });
  if (state.closed || socket.protocol !== protocol) throw Error();
  socket.onclose = () => terminal(deps.document, state); socket.onerror = () => terminal(deps.document, state);
  const mod = await deps.importModule("/accelerator/browser/assets/browser.js");
  if (state.closed || !exactObject(mod,["mountAcceleratorBrowser"]) || typeof mod.mountAcceleratorBrowser !== "function") throw Error();
  const request = async (method,args) => {
   if (state.closed || !state.bearer) throw Error();
   try {
    const response = await deps.fetch("/api/call", {method:"POST",headers:{"Content-Type":"application/json",Authorization:"Bearer "+state.bearer},body:JSON.stringify({method,args}),credentials:"omit",cache:"no-store",redirect:"error",referrerPolicy:"no-referrer"});
    if (state.closed || !response.ok) throw Error();
    const body=await response.json();
    if (state.closed || !exactObject(body,["data"])) throw Error();
    return body.data;
   } catch {
    terminal(deps.document,state);
    throw Error();
   }
  };
  socket.onmessage = event => { try { const frame=JSON.parse(event.data); if (!exactObject(frame,["type","name","data"]) || frame.type !== "event" || !eventNames.has(frame.name)) throw Error(); for (const listener of state.listeners) listener({type:frame.name,data:frame.data}); } catch { terminal(deps.document,state); } };
  const facade = Object.freeze({ListSecretsMetadata:(...a)=>request("ListSecretsMetadata",a),GetSecretData:(...a)=>request("GetSecretData",a),GetSecretYaml:(...a)=>request("GetSecretYaml",a),CancelListRequest:(...a)=>request("CancelListRequest",a),SubscribeSecretWatcher:(...a)=>request("SubscribeSecretWatcher",a),UnsubscribeSecretWatcher:(...a)=>request("UnsubscribeSecretWatcher",a),events:Object.freeze({subscribe(fn){if (state.closed) return ()=>{}; state.listeners.add(fn);return()=>state.listeners.delete(fn)}}),close:()=>terminal(deps.document,state)});
  if (state.closed) throw Error(); const cleanup = mod.mountAcceleratorBrowser(facade); if (typeof cleanup !== "function") throw Error(); if (state.closed) { cleanup(); throw Error(); } state.cleanup = cleanup;
  if (state.closed) throw Error();
 } catch { terminal(deps.document, state); }
 return state;
}
if (typeof window !== "undefined") start({document:window.document, location:window.location, history:window.history, fetch:window.fetch.bind(window), WebSocket:window.WebSocket, importModule:path=>import(path)});
