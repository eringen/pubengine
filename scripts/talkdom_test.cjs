const assert = require('assert');
const fs = require('fs');
const vm = require('vm');
const events = {}, pending = [];
const location = { pathname: '/', search: '', href: 'https://example.test/', reload() { this.reloaded = true; }, assign(url) { this.assigned = url; } };
const receiver = { innerHTML: 'home', isConnected: true, getAttribute: n => n === 'receiver' ? 'content' : null, hasAttribute: n => n === 'receiver', dispatchEvent() {} };
const history = { state: null, replaceState(state) { this.state = state; }, pushState(state, _, url) { this.state = state; location.pathname = url; location.href = new URL(url, location.href).href; } };
const context = {
  location, history, console, URL, AbortController, Map,
  window: { addEventListener: (n, f) => events[n] = f },
  document: { readyState: 'complete', addEventListener: (n, f) => events[n] = f, querySelector: () => null, querySelectorAll: q => q === '[receiver]' ? [receiver] : [] },
  fetch: url => new Promise(resolve => pending.push({ url, resolve })),
  CustomEvent: function(n, o) { this.type = n; this.detail = o.detail; }, setInterval, clearInterval,
};
vm.runInNewContext(fs.readFileSync(__dirname + '/../embedded/talkdom.js', 'utf8'), context);
const flush = () => new Promise(setImmediate);
const response = text => ({ ok: true, headers: { get: () => null }, text: () => Promise.resolve(text) });
(async () => {
  assert(history.state.sender.includes('content get: /'));
  const first = context.window.talkDOM.send('content get: /slow apply: inner').catch(e => e);
  const second = context.window.talkDOM.send('content get: /fast apply: inner');
  await flush();
  pending[1].resolve(response('newer')); await second;
  pending[0].resolve(response('stale')); await first;
  assert.equal(receiver.innerHTML, 'newer');
  const sender = { hasAttribute: n => n === 'push-url', getAttribute: n => n === 'sender' ? 'content get: /failed apply: inner' : n === 'push-url' ? '/failed/' : null };
  events.click({ target: { closest: () => sender }, preventDefault() {} });
  assert.equal(location.pathname, '/');
  await flush(); pending[2].resolve({ ok: false, status: 500 }); await flush();
  assert.equal(location.pathname, '/');
  const before = pending.length;
  events.click({ ctrlKey: true, target: { closest: () => sender }, preventDefault() { throw Error('modified click intercepted'); } });
  await flush(); assert.equal(pending.length, before);
  events.popstate({ state: null }); assert(location.reloaded);
  console.log('Navigation ordering and history tests passed');
})().catch(e => { console.error(e); process.exitCode = 1; });
