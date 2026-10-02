"use strict";
(function () {
  if (["1", "yes"].includes(navigator.doNotTrack || window.doNotTrack || "")) return;
  var script = document.currentScript;
  var endpoint = (script && script.src ? new URL(script.src).origin : "") + "/api/analytics/collect";
  var active = null;
  function excluded(path) { return path === "/admin" || path.indexOf("/admin/") === 0; }
  if (excluded(window.location.pathname)) return;
  function duration() { return active.elapsed + (active.since === null ? 0 : Date.now() - active.since); }

  function newID() {
    if (window.crypto && window.crypto.getRandomValues) {
      var bytes = new Uint8Array(16);
      window.crypto.getRandomValues(bytes);
      return Array.from(bytes, function (b) { return b.toString(16).padStart(2, "0"); }).join("");
    }
    return Date.now().toString(36) + "_" + Math.random().toString(36).slice(2) + Math.random().toString(36).slice(2);
  }

  function referrer() {
    try {
      var source = document.referrer;
      return source && new URL(source).host !== window.location.host ? source : "";
    } catch (_) { return ""; }
  }

  function send(event) {
    if (!active) return;
    var body = JSON.stringify({
      event: event,
      page_view_id: active.id,
      path: active.path,
      referrer: active.referrer,
      screen_size: screen.width + "x" + screen.height,
      user_agent: navigator.userAgent,
      duration_sec: event === "view" ? 0 : Math.min(86400, Math.max(0, Math.round(duration() / 1000)))
    });
    if (typeof navigator.sendBeacon === "function" && navigator.sendBeacon(endpoint, new Blob([body], { type: "application/json" }))) return;
    fetch(endpoint, { method: "POST", headers: { "Content-Type": "application/json" }, body: body, keepalive: true }).catch(function () {});
  }

  function start(path) {
    path = typeof path === "string" ? path : window.location.pathname;
    if (excluded(path)) { active = null; return; }
    active = { id: newID(), path: path, referrer: referrer(), elapsed: 0, since: document.hidden ? null : Date.now(), ended: false };
    send("view");
  }

  function finish() {
    if (!active || active.ended) return;
    send("duration");
    active.ended = true;
  }

  function navigate(event) {
    if (!event.detail || event.detail.receiver !== "content") return;
    // The URL is already updated, but the outgoing view retains its original path.
    var path = event.detail.url ? new URL(event.detail.url, window.location.href).pathname : window.location.pathname;
    if (active && active.path === path) return;
    finish();
    start(path);
  }

  document.addEventListener("talkdom:done", navigate);
  document.addEventListener("talkdom:navigate", navigate);
  document.addEventListener("visibilitychange", function () {
    if (!active || active.ended) return;
    if (document.hidden) {
      active.elapsed = duration(); active.since = null; send("duration");
    } else if (active.since === null) { active.since = Date.now(); }
  });
  window.addEventListener("pagehide", finish);
  window.addEventListener("beforeunload", finish);
  window.addEventListener("pageshow", function (event) { if (event.persisted) start(); });
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start);
  else start();
  window.Nanolytica = { track: function () { finish(); start(); } };
})();
