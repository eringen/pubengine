// app.js — client-side behavior for pubengine
// Handles HTMX navigation, scroll restoration, and dark mode toggle.

// After HTMX swaps main-content, scroll to top for posts or to hash target for home.
document.addEventListener("htmx:afterSwap", (e) => {
  if (e.detail.target.id === "main-content") {
    const isPost = window.location.pathname.startsWith("/blog/");
    if (isPost) {
      window.scrollTo(0, 0);
    } else if (window.location.hash) {
      const el = document.querySelector(window.location.hash);
      if (el) el.scrollIntoView();
    }
  }
});

// Handle browser back/forward — fetch the correct partial via HTMX.
window.addEventListener("popstate", () => {
  const path = window.location.pathname;
  const mainContent = document.getElementById("main-content");
  if (!mainContent) {
    window.location.reload();
    return;
  }

  let url;
  if (path.startsWith("/blog/")) {
    url = path + "/?partial=post";
  } else {
    url = "/?partial=home" + window.location.search.replace("?", "&");
  }

  htmx.ajax("GET", url, { target: "#main-content", swap: "innerHTML" });
});

// Dark mode toggle — persists choice in localStorage.
document.addEventListener("DOMContentLoaded", () => {
  const toggle = document.getElementById("theme-toggle");
  const iconLight = document.getElementById("theme-icon-light");
  const iconDark = document.getElementById("theme-icon-dark");

  function isDark() {
    return document.documentElement.classList.contains("dark");
  }

  function updateIcons() {
    if (iconLight && iconDark) {
      iconLight.classList.toggle("hidden", !isDark());
      iconDark.classList.toggle("hidden", isDark());
    }
  }

  updateIcons();

  if (toggle) {
    toggle.addEventListener("click", () => {
      if (isDark()) {
        document.documentElement.classList.remove("dark");
        localStorage.setItem("theme", "light");
      } else {
        document.documentElement.classList.add("dark");
        localStorage.setItem("theme", "dark");
      }
      updateIcons();
    });
  }
});
