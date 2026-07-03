"use strict";
(function () {
  if (["1", "yes"].includes(navigator.doNotTrack || window.doNotTrack || "")) return;
  var script = document.currentScript;
  var endpoint = (script && script.src ? new URL(script.src).origin : "") + "/api/analytics/collect";
  var active = null;

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
      duration_sec: event === "view" ? 0 : Math.min(86400, Math.max(0, Math.round((Date.now() - active.started) / 1000)))
    });
    if (typeof navigator.sendBeacon === "function" && navigator.sendBeacon(endpoint, new Blob([body], { type: "application/json" }))) return;
    fetch(endpoint, { method: "POST", headers: { "Content-Type": "application/json" }, body: body, keepalive: true }).catch(function () {});
  }

  function start() {
    active = { id: newID(), path: window.location.pathname, referrer: referrer(), started: Date.now(), ended: false };
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
    if (active && active.path === window.location.pathname) return;
    finish();
    start();
  }

  document.addEventListener("talkdom:done", navigate);
  window.addEventListener("pagehide", finish);
  window.addEventListener("beforeunload", finish);
  window.addEventListener("pageshow", function (event) { if (event.persisted) start(); });
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start);
  else start();
  window.Nanolytica = { track: function () { finish(); start(); } };
})();
