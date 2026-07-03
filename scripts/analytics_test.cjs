const assert = require("assert");
const fs = require("fs");
const vm = require("vm");

const events = {}, requests = [];
let now = 1000;
class BlobMock { constructor(parts) { this.body = parts.join(""); } }
const context = {
  Blob: BlobMock, URL, Uint8Array, Array, Math, Date: { now: () => now },
  screen: { width: 100, height: 100 },
  navigator: { userAgent: "test", sendBeacon: (_, blob) => { requests.push(JSON.parse(blob.body)); return true; } },
  window: { location: { pathname: "/a/", host: "example.com" }, addEventListener: (name, fn) => events[name] = fn },
  document: { referrer: "", readyState: "complete", currentScript: { src: "https://example.com/public/analytics.js" }, addEventListener: (name, fn) => events[name] = fn },
};
vm.runInNewContext(fs.readFileSync(__dirname + "/../embedded/analytics.js", "utf8"), context);
now = 11000;
context.window.location.pathname = "/b/";
events["talkdom:done"]({ detail: { receiver: "content" } });
assert.deepStrictEqual(requests.map(r => [r.event, r.path, r.duration_sec]), [["view", "/a/", 0], ["duration", "/a/", 10], ["view", "/b/", 0]]);
assert.strictEqual(requests[0].page_view_id, requests[1].page_view_id);
assert.notStrictEqual(requests[0].page_view_id, requests[2].page_view_id);
events.pagehide(); events.beforeunload();
assert.strictEqual(requests.length, 4);
assert.strictEqual(requests[3].event, "duration");
assert.strictEqual(requests[3].duration_sec, 0);
events.pageshow({ persisted: true });
assert.strictEqual(requests.length, 5);
assert.notStrictEqual(requests[4].page_view_id, requests[2].page_view_id);
events.pagehide();
assert.strictEqual(requests.length, 6);
console.log("Analytics navigation and lifecycle tests passed");
