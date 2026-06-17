(function () {

  function parseMessage(str) {
    var trimmed = str.trim();
    var tokens = trimmed.split(/\s+/);
    var receiver = tokens[0];
    var body = trimmed.substring(receiver.length).trim();
    var rest = tokens.slice(1);
    var keywords = [];
    var args = [];
    var currentArg = [];

    for (var i = 0; i < rest.length; i++) {
      var token = rest[i];
      if (token.endsWith(":")) {
        if (keywords.length > 0 && currentArg.length > 0) {
          args.push(currentArg.join(" "));
          currentArg = [];
        } else if (keywords.length > 0) {
          args.push("");
        }
        keywords.push(token);
      } else {
        currentArg.push(token);
      }
    }
    if (keywords.length > 0) {
      args.push(currentArg.join(" "));
    }

    return { receiver: receiver, selector: keywords.join(""), keywords: keywords, args: args, body: body };
  }

  function receiverName(el) {
    return el.getAttribute("receiver").trim().split(/\s+/)[0];
  }

  function findReceivers(name) {
    return Array.from(document.querySelectorAll('[receiver]')).filter(function (el) {
      return el.getAttribute('receiver').split(/\s+/).includes(name);
    });
  }

  function accepts(el, op) {
    var attr = el.getAttribute("accepts");
    if (!attr) return true;
    return attr.split(/\s+/).indexOf(op) !== -1;
  }

  function persist(el, op) {
    if (!el.hasAttribute("receiver") || !el.hasAttribute("persist")) return;
    var name = receiverName(el);
    var key = "talkDOM:" + name;
    if (op === "outer") {
      localStorage.setItem(key, JSON.stringify({ op: op, content: el.outerHTML }));
    } else {
      localStorage.setItem(key, JSON.stringify({ op: op, content: el.innerHTML }));
    }
  }

  function restore() {
    document.querySelectorAll("[persist]").forEach(function (el) {
      if (!el.hasAttribute("receiver")) return;
      var name = receiverName(el);
      var raw = localStorage.getItem("talkDOM:" + name);
      if (!raw) return;
      var state = JSON.parse(raw);
      if (state.op === "outer") {
        el.outerHTML = state.content;
      } else {
        el.innerHTML = state.content;
      }
    });
  }

  function apply(el, op, content) {
    if (!accepts(el, op)) {
      console.error(receiverName(el) + " does not accept " + op);
      return;
    }
    if (op === "outer" && receiverName(el) === "content") {
      var page = new DOMParser().parseFromString(content, "text/html");
      var main = page.querySelector('[receiver~="content"]');
      if (main) {
        content = main.outerHTML;
        var selectors = 'title,meta[name="description"],meta[property^="og:"],link[rel="canonical"],script[type="application/ld+json"]';
        document.querySelectorAll(selectors).forEach(function (node) { node.remove(); });
        page.querySelectorAll(selectors).forEach(function (node) { document.head.appendChild(node.cloneNode(true)); });
      }
    }
    var name = recName(el);
    switch (op) {
      case "inner": el.innerHTML = content; break;
      case "text": el.textContent = content; break;
      case "append": el.insertAdjacentHTML("beforeend", content); break;
      case "outer": el.outerHTML = content; break;
    }
    persist(op === "outer" ? (findReceivers(name)[0] || el) : el, op);
  }

  function csrfToken() {
    var meta = document.querySelector('meta[name="csrf-token"]');
    return meta ? meta.getAttribute("content") : "";
  }

  var requests = new Map();

  function request(method, url, receiver) {
    var previous = requests.get(receiver);
    if (previous) previous.abort();
    var controller = new AbortController();
    requests.set(receiver, controller);
    var headers = { "X-TalkDOM-Request": "true", "X-TalkDOM-Current-URL": location.href };
    if (receiver) headers["X-TalkDOM-Receiver"] = receiver;
    if (method !== "GET") {
      var token = csrfToken();
      if (token) headers["X-CSRF-Token"] = token;
    }
    return fetch(url, { method: method, headers: headers, signal: controller.signal }).then(async function (r) {
      if (r.redirected && new URL(r.url, location.href).pathname === "/admin/") {
        location.assign(r.url);
        throw new Error("Authentication required");
      }
      if (!r.ok) throw new Error("HTTP " + r.status);
      var text = await r.text();
      if (controller.signal.aborted || requests.get(receiver) !== controller) throw new Error("Superseded request");
      var trigger = r.headers.get("X-TalkDOM-Trigger");
      if (trigger) dispatchRaw(trigger);
      return text;
    }).finally(function () {
      if (requests.get(receiver) === controller) requests.delete(receiver);
    });
  }

  function requestApply(method, el, url, op) {
    var name = recName(el);
    return request(method, url, name).then(function (text) {
      findReceivers(name).forEach(function (target) { apply(target, op, text); });
    });
  }

  function recName(el) {
    return el.hasAttribute("receiver") ? receiverName(el) : "";
  }

  const methods = {
    "get:": function (el, url) { return request("GET", url, recName(el)); },
    "post:": function (el, url) { return request("POST", url, recName(el)); },
    "put:": function (el, url) { return request("PUT", url, recName(el)); },
    "delete:": function (el, url) { return request("DELETE", url, recName(el)); },
    "confirm:": function (el, message) { if (!confirm(message)) return Promise.reject(); },
    "apply:": function (el, content, op) { apply(el, op, content); },
    "get:apply:": function (el, url, op) { return requestApply("GET", el, url, op); },
    "post:apply:": function (el, url, op) { return requestApply("POST", el, url, op); },
    "put:apply:": function (el, url, op) { return requestApply("PUT", el, url, op); },
    "delete:apply:": function (el, url, op) { return requestApply("DELETE", el, url, op); },
  };

  function pushUrl(senderEl, raw) {
    if (!senderEl.hasAttribute("push-url")) return;
    var url = senderEl.getAttribute("push-url");
    if (!url) {
      var firstMsg = parseMessage(raw.split(";")[0].split("|")[0].trim());
      url = firstMsg.args[0] || "";
    }
    if (url && (location.pathname + location.search) !== url) {
      history.pushState({ sender: raw, url: url }, "", url);
    }
  }

  function completed(raw) {
    var receiver = parseMessage(raw.split(";")[0].split("|")[0]).receiver;
    var target = findReceivers(receiver)[0];
    if (target) target.dispatchEvent(new CustomEvent("talkdom:done", { bubbles: true, detail: { receiver: receiver, url: location.href } }));
  }

  function replayState(state) {
    if (!state || !state.sender) { location.reload(); return; }
    run(state.sender, true).then(function () { completed(state.sender); }).catch(function () { location.reload(); });
  }

  window.addEventListener("popstate", function (e) { replayState(e.state); });

  function send(msg, piped, silent) {
    var els = findReceivers(msg.receiver);
    if (els.length === 0) {
      return Promise.reject(new Error(msg.receiver + " not found"));
    }
    var method = methods[msg.selector];
    if (!method) {
      return Promise.reject(new Error("Unknown command " + msg.selector));
    }
    var args = piped !== undefined ? [piped].concat(msg.args) : msg.args;
    var detail = { receiver: msg.receiver, selector: msg.selector, args: msg.args };
    var result;
    if (/^(get|post|put|delete):/.test(msg.selector)) els = Array.from(els).slice(0, 1);
    var results = [];
    els.forEach(function (el) {
      result = Promise.resolve().then(function () { return method(el, ...args); });
      results.push(result);
      if (result && typeof result.then === "function") {
        result.then(function () {
          var target = el.isConnected ? el : findReceivers(msg.receiver)[0];
          if (target && !silent) target.dispatchEvent(new CustomEvent("talkdom:done", { bubbles: true, detail: detail }));
        }, function (err) {
          detail.error = err;
          var target = el.isConnected ? el : findReceivers(msg.receiver)[0];
          if (target) target.dispatchEvent(new CustomEvent("talkdom:error", { bubbles: true, detail: detail }));
        });
      } else {
        var target = el.isConnected ? el : findReceivers(msg.receiver)[0];
        if (target) target.dispatchEvent(new CustomEvent("talkdom:done", { bubbles: true, detail: detail }));
      }
    });
    return Promise.all(results).then(function (values) { return values[values.length - 1]; });
  }

  function run(raw, silent) {
    var chains = raw.split(";").map(function (chain) {
      var trimmed = chain.trim();
      if (!trimmed) return Promise.resolve();
      var steps = trimmed.split("|").map(function (s) { return s.trim(); }).filter(Boolean);
      if (steps.length === 1) {
        return Promise.resolve(send(parseMessage(steps[0]), undefined, silent));
      }
      return steps.reduce(function (prev, step) {
        var msg = parseMessage(step);
        return Promise.resolve(prev).then(function (piped) {
          return send(msg, piped, silent);
        });
      }, undefined);
    });
    return Promise.all(chains);
  }

  function dispatchRaw(raw) {
    run(raw).catch(function () {});
  }

  function dispatch(senderEl) {
    var raw = senderEl.getAttribute("sender");
    run(raw, true).then(function () {
      pushUrl(senderEl, raw);
      completed(raw);
    }).catch(function () {});
  }

  function parseInterval(str) {
    var match = str.match(/^(\d+)(s|ms)$/);
    if (!match) return null;
    var n = parseInt(match[1], 10);
    return match[2] === "s" ? n * 1000 : n;
  }

  function startPolling(el) {
    var attr = el.getAttribute("receiver");
    var msg = parseMessage(attr);
    if (msg.keywords[msg.keywords.length - 1] !== "poll:") return;
    var interval = parseInterval(msg.args[msg.args.length - 1]);
    if (!interval) {
      console.error("poll: invalid interval for " + msg.receiver);
      return;
    }
    var selector = msg.keywords.slice(0, -1).join("");
    var args = msg.args.slice(0, -1);
    var name = msg.receiver;
    var id = setInterval(function () {
      if (!el.isConnected) { clearInterval(id); return; }
      var targets = findReceivers(name);
      if (targets.length === 0) return;
      var method = methods[selector];
      if (!method) {
        console.error(name + " does not understand " + selector);
        return;
      }
      targets.forEach(function (target) { method(target, ...args); });
    }, interval);
  }

  document.addEventListener("click", function (e) {
    const sender = e.target.closest("[sender]");
    if (sender && !e.defaultPrevented && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey && (!e.button || e.button === 0) && !sender.hasAttribute("download") && (!sender.getAttribute("target") || sender.getAttribute("target") === "_self")) {
      e.preventDefault();
      dispatch(sender);
    }
  });

  function initialize() {
    try { restore(); } catch (_) {}
    if (!history.state || !history.state.sender) {
      history.replaceState({ sender: "content get: " + location.pathname + location.search + " apply: outer", url: location.href }, "", location.href);
    }
    document.querySelectorAll("[receiver]").forEach(startPolling);
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", initialize);
  else initialize();

  window.talkDOM = { methods: methods, send: run };

}());
