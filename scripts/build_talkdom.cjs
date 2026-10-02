// Keep the npm release intact; apply PubEngine's documented integration at build time.
// Generic request/click fixes also live in TalkDOM commit c351966 (not yet published).
// Exact-match replacements fail loudly when an upstream upgrade changes these seams.
const fs = require('node:fs');
const path = require('node:path');
const { transformSync } = require('esbuild');
let source = fs.readFileSync(require.resolve('talkdom'), 'utf8');
function replace(before, after) {
  if (!source.includes(before)) throw new Error('TalkDOM integration no longer matches: ' + before.slice(0, 80));
  source = source.replace(before, after);
}

replace('  var csrfMeta = null;', `  var csrfMeta = null;
  var requests = new WeakMap();
  var pendingReads = new Set();
  function abortReads() {
    pendingReads.forEach(function (controller) { controller.abort(); });
    pendingReads.clear();
  }
  var metadataSelector = 'title,meta[name="description"],meta[property^="og:"],link[rel="canonical"],script[type="application/ld+json"]';
  function metadataSnapshot() {
    return Array.from(document.querySelectorAll(metadataSelector), function (node) { return node.outerHTML; }).join('');
  }
  function restoreMetadata(html) {
    if (typeof html !== 'string') return;
    var page = new DOMParser().parseFromString(html, 'text/html');
    document.querySelectorAll(metadataSelector).forEach(function (node) { node.remove(); });
    page.querySelectorAll(metadataSelector).forEach(function (node) { document.head.appendChild(node.cloneNode(true)); });
  }
  function navigated(traversal = false) {
    var target = findReceivers('content')[0];
    if (target) target.dispatchEvent(new CustomEvent('talkdom:navigate', {
      bubbles: true, detail: { receiver: 'content', url: location.href, traversal: traversal }
    }));
  }`);
replace('    switch (op) {', `    if (op === 'outer' && receiverName(el) === 'content') {
      var page = new DOMParser().parseFromString(String(content), 'text/html');
      var main = page.querySelector('[receiver~="content"]');
      if (main) {
        content = main.outerHTML;
        // Fragments without a document head must not erase the current metadata.
        if (page.querySelector('title')) restoreMetadata(Array.from(page.querySelectorAll(metadataSelector), function (node) { return node.outerHTML; }).join(''));
      }
    }
    switch (op) {`);
replace('function request(method, url, receiver) {', 'function request(method, url, receiver, el) {');
replace('    return fetch(url, { method: method, headers: headers }).then(function (r) {', `    var controller = method === "GET" ? new AbortController() : null;
    if (controller) {
      var previous = requests.get(el);
      if (previous) previous.abort();
      requests.set(el, controller);
      pendingReads.add(controller);
    }
    function cleanup() {
      if (!controller) return;
      pendingReads.delete(controller);
      if (requests.get(el) === controller) requests.delete(el);
    }
    var result;
    try { result = fetch(url, { method: method, headers: headers, signal: controller ? controller.signal : undefined }); }
    catch (err) { cleanup(); return Promise.reject(err); }
    return Promise.resolve(result).then(function (r) {
      if (r.redirected && new URL(r.url, location.href).pathname === '/admin/') {
        location.assign(r.url);
        throw new Error('Authentication required');
      }`);
replace('      return r.text().then(function (text) {', `      return r.text().then(function (text) {
        if (controller && (controller.signal.aborted || requests.get(el) !== controller)) throw new DOMException('Superseded request', 'AbortError');`);
replace('      return Promise.reject(err);\n    });\n  }\n\n  function recName', `      return Promise.reject(err);
    }).finally(cleanup);
  }

  function recName`);
source = source.replaceAll('recName(el))', 'recName(el), el)');
replace('talkDOM: { version: 1, regions: historySnapshot() }', 'talkDOM: { version: 1, regions: historySnapshot(), metadata: metadataSnapshot() }');
replace('    ensureHistoryRegions();\n    saved.regions.forEach', '    ensureHistoryRegions();\n    restoreMetadata(saved.metadata);\n    saved.regions.forEach');
replace('    navigation++;\n    restoreHistory(e.state);', `    navigation++;
    abortReads();
    if (!e.state || !e.state.talkDOM) { location.reload(); return; }
    restoreHistory(e.state);
    navigated(true);`);
replace('    var current = ++navigation;', '    var current = ++navigation;\n    abortReads();');
replace('      else history.pushState(historyState(), "", url);', '      else history.pushState(historyState(), "", url);\n      navigated();');
replace('    if (sender) {', `    if (sender && !e.defaultPrevented && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey && e.button === 0 && !sender.hasAttribute('download') && (!sender.getAttribute('target') || sender.getAttribute('target') === '_self')) {`);

replace('    config: config,', '    config: config,\n    pubengineNavigationEvents: true,');

const license = fs.readFileSync(path.join(path.dirname(require.resolve('talkdom')), 'LICENSE'), 'utf8');
const output = '/*! talkDOM 0.5.0 + PubEngine integration. Generated by scripts/build_talkdom.cjs.\n' + license + '*/\n' + transformSync(source, { minify: true, target: 'es2020' }).code;
const destination = path.join(__dirname, '../embedded/talkdom.js');
if (process.argv.includes('--check')) {
  if (fs.readFileSync(destination, 'utf8') !== output) throw new Error('Run npm run build:talkdom and commit embedded/talkdom.js');
} else fs.writeFileSync(destination, output);
