// kg-ui boot — wires the shared core, kicks off per-tab modules
// lazily on first activation, and respects the hash-route default.

(function () {
  const tabBooted = { explore: false, monitor: false, queue: false, insights: false };

  function bootTab(name) {
    if (tabBooted[name]) {
      // Already booted — fire a re-init hook for tabs that need it
      // (e.g. graph engine resize after the panel becomes visible).
      const fn = window["kg" + cap(name) + "Init"];
      if (typeof fn === "function") { try { fn(); } catch (_) {} }
      return;
    }
    tabBooted[name] = true;
    const fn = window["kg" + cap(name) + "Init"];
    if (typeof fn === "function") fn();
  }
  function cap(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

  // Core boots immediately (header pills + SSE + theme). Then we
  // activate whichever tab the URL hash points at; tabs lazy-init.
  window.kgCoreInit();

  const start = window.kg.currentTab();
  window.kg.activateTab(start);
  bootTab(start);

  // Subsequent tab switches: bootTab fires on the tab:show event.
  window.addEventListener("tab:show", (ev) => {
    if (ev.detail && ev.detail.tab) bootTab(ev.detail.tab);
  });
})();
