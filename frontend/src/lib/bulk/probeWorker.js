// Worker source for the bulk capability probe. It is loaded two ways: as text
// into a Blob worker, and as a same-origin script URL worker. Keep it free of
// imports and syntax that needs transpiling.
self.onmessage = function (ev) {
    var msg = ev.data || {};
    if (msg.type === 'hello') {
        self.postMessage({ type: 'hello', ok: true });
        return;
    }
    if (msg.type === 'fetch') {
        fetch(msg.url, { method: 'GET', headers: msg.headers, cache: 'no-store' })
            .then(function (r) {
                return r.json().then(function (j) {
                    self.postMessage({ type: 'fetch', ok: r.ok && j && j.ok === true, status: r.status });
                });
            })
            .catch(function (e) {
                self.postMessage({ type: 'fetch', ok: false, error: String((e && e.message) || e) });
            });
    }
};
