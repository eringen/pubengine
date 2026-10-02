const assert = require('node:assert/strict');
const fs = require('node:fs');
const { test } = require('node:test');
const { JSDOM, VirtualConsole } = require('jsdom');
const source = fs.readFileSync(__dirname + '/../embedded/talkdom.js', 'utf8');
const flush = () => new Promise(setImmediate);
const response = text => ({ ok: true, headers: { get: () => null }, text: async () => text });

async function setup(t, extra = '') {
  const dom = new JSDOM('<!doctype html><title>Home</title><meta name="description" content="Home description"><main receiver="content">Home</main>' + extra, {
    url: 'https://example.test/', runScripts: 'outside-only', virtualConsole: new VirtualConsole(),
  });
  t.after(() => dom.window.close());
  const pending = [];
  dom.window.fetch = (url, options) => new Promise(resolve => pending.push({ url, options, resolve }));
  dom.window.eval(source);
  await flush();
  return { window: dom.window, document: dom.window.document, pending, api: dom.window.talkDOM };
}

test('latest response wins and failures preserve content', async t => {
  const { api, document, pending } = await setup(t);
  const first = api.send('content get: /slow apply: inner').catch(e => e);
  const second = api.send('content get: /fast apply: inner');
  assert(pending[0].options.signal.aborted);
  pending[1].resolve(response('newer')); await second;
  pending[0].resolve(response('stale')); await first;
  assert.equal(document.querySelector('main').textContent, 'newer');
  const failed = api.send('content get: /failed apply: inner');
  pending[2].resolve({ ok: false, status: 503 });
  await assert.rejects(failed);
  assert.equal(document.querySelector('main').textContent, 'newer');
});

test('document navigation restores metadata and emits committed URLs on Back and Forward', async t => {
  const { window, document, pending } = await setup(t, '<a sender="content get: /post apply: outer" push-url="/post/">Read</a>');
  const visits = [];
  document.addEventListener('talkdom:navigate', e => visits.push(e.detail.url));
  document.querySelector('a').click();
  pending[0].resolve(response('<!doctype html><title>Post</title><meta name="description" content="Post description"><link rel="canonical" href="https://example.test/post/"><main receiver="content">Article</main><script type="application/ld+json">{"name":"Post"}</script>'));
  await flush();
  assert.equal(window.location.pathname, '/post/');
  assert.equal(document.title, 'Post');
  assert.equal(document.querySelectorAll('main').length, 1);
  assert.equal(document.querySelectorAll('html').length, 1);
  assert.deepEqual(visits, ['https://example.test/post/']);
  const back = new Promise(resolve => window.addEventListener('popstate', resolve, { once: true }));
  window.history.back(); await back;
  assert.equal(document.title, 'Home');
  assert.equal(document.querySelector('meta[name=description]').content, 'Home description');
  assert.equal(document.querySelector('main').textContent, 'Home');
  assert.equal(document.querySelector('script[type="application/ld+json"]'), null);
  const forward = new Promise(resolve => window.addEventListener('popstate', resolve, { once: true }));
  window.history.forward(); await forward;
  assert.equal(document.title, 'Post');
  assert.equal(document.querySelector('main').textContent, 'Article');
  assert.equal(pending.length, 1, 'history snapshots should not refetch pages');
  assert.equal(visits.length, 3);
});

test('failed navigation preserves history; modified clicks retain browser behavior', async t => {
  const { window, document, pending } = await setup(t, '<a href="/post/" sender="content get: /post apply: outer" push-url="/post/">Read</a>');
  const link = document.querySelector('a');
  const modified = new window.MouseEvent('click', { bubbles: true, cancelable: true, ctrlKey: true });
  link.dispatchEvent(modified);
  assert.equal(modified.defaultPrevented, false);
  assert.equal(pending.length, 0);
  link.click();
  pending[0].resolve({ ok: false, status: 503 });
  await flush();
  assert.equal(window.location.pathname, '/');
  assert.equal(window.history.length, 1);
  assert.equal(document.title, 'Home');
});

test('receiver groups, same-turn replacements, and strict applies retain upstream behavior', async t => {
  const { api, document, pending } = await setup(t, '<div receiver="group alias"></div><div receiver="group"></div>');
  const send = api.send('group get: /fragment apply: inner');
  assert.equal(pending.length, 2);
  pending.forEach(p => p.resolve(response('updated'))); await send;
  assert.deepEqual(Array.from(api.receivers('group'), el => el.textContent), ['updated', 'updated']);
  await api.send('alias text: literal');
  document.querySelector('[receiver="group alias"]').setAttribute('receiver', 'renamed');
  await api.send('renamed text: replacement');
  assert.equal(api.receivers('renamed')[0].textContent, 'replacement');
  api.config.strictApply = true;
  const el = api.receivers('renamed')[0]; el.setAttribute('accepts', 'text');
  await assert.rejects(api.deliver(el, 'apply:', ['denied', 'inner']));
  assert.equal(el.textContent, 'replacement');
});

test('cross-origin writes do not include CSRF or current-page headers', async t => {
  const { api, pending } = await setup(t, '<meta name="csrf-token" content="private">');
  const call = api.send('content post: https://other.test/save');
  assert.equal(pending[0].options.headers['X-CSRF-Token'], undefined);
  assert.equal(pending[0].options.headers['X-TalkDOM-Current-URL'], undefined);
  pending[0].resolve(response('')); await call;
});
