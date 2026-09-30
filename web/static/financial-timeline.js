(() => {
  "use strict";

  const dialog = document.getElementById("financial-timeline-modal");
  if (!dialog) return;

  const stateKey = "fmlyFinancialTimelineStateV1";
  let returnFocus = null;

  function loadState() {
    try {
      const state = JSON.parse(sessionStorage.getItem(stateKey) || "null");
      if (!state || !Number.isInteger(state.page) || state.page < 1) return null;
      return state;
    } catch {
      return null;
    }
  }

  function saveState(preferredKey = "") {
    const page = Number(dialog.dataset.page) || 1;
    const overlayTop = dialog.getBoundingClientRect().top;
    const headerBottom = dialog.querySelector(".financial-timeline-header").getBoundingClientRect().bottom;
    const entries = [...dialog.querySelectorAll("[data-timeline-key]")];
    let anchor = preferredKey
      ? entries.find((entry) => entry.dataset.timelineKey === preferredKey)
      : null;
    if (!anchor) anchor = entries.find((entry) => entry.getBoundingClientRect().bottom > headerBottom) || entries[0];
    const rect = anchor?.getBoundingClientRect();
    const state = {
      page,
      scrollTop: dialog.scrollTop,
      anchorKey: anchor?.dataset.timelineKey || "",
      anchorOffset: rect ? rect.top - overlayTop : 0,
    };
    try {
      sessionStorage.setItem(stateKey, JSON.stringify(state));
    } catch {
      // Keep minimizing and record navigation available if storage is disabled.
    }
    return state;
  }

  function restorePosition(state, page) {
    if (!state || state.page !== page) return;
    requestAnimationFrame(() => requestAnimationFrame(() => {
      if (dialog.hidden) return;
      const entry = state.anchorKey
        ? dialog.querySelector(`[data-timeline-key="${CSS.escape(state.anchorKey)}"]`)
        : null;
      if (entry && Number.isFinite(state.anchorOffset)) {
        const delta = entry.getBoundingClientRect().top - dialog.getBoundingClientRect().top - state.anchorOffset;
        dialog.scrollTop += delta;
      } else if (Number.isFinite(state.scrollTop)) {
        dialog.scrollTop = state.scrollTop;
      }
    }));
  }

  function hasOpenParameter() {
    const url = new URL(window.location.href);
    return url.pathname === "/assets" && url.searchParams.get("timeline") === "1";
  }

  function setOpen(open, focus = true) {
    dialog.hidden = !open;
    document.body.classList.toggle("financial-timeline-open", open);
    if (open && focus) {
      returnFocus = document.activeElement;
      dialog.querySelector("[data-close-financial-timeline]").focus();
    } else if (open && !returnFocus) {
      returnFocus = document.querySelector("[data-open-financial-timeline]");
    } else if (!open && focus && returnFocus && returnFocus.isConnected) {
      returnFocus.focus();
    }
  }

  function setOpenURL(page) {
    const url = new URL(window.location.href);
    url.searchParams.set("timeline", "1");
    url.searchParams.set("page", String(page));
    window.history.pushState({}, "", url);
  }

  function clearOpenURL() {
    const url = new URL(window.location.href);
    url.searchParams.delete("timeline");
    url.searchParams.delete("page");
    window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
  }

  function minimizeTimeline() {
    saveState();
    clearOpenURL();
    setOpen(false);
  }

  function closeTimeline() {
    clearOpenURL();
    setOpen(false);
  }

  function openTimeline(event) {
    if (event) event.preventDefault();
    const state = loadState();
    const page = state?.page || 1;
    const renderedPage = Number(dialog.dataset.page) || 1;
    if (page !== renderedPage) {
      const url = new URL("/assets", window.location.origin);
      url.searchParams.set("timeline", "1");
      url.searchParams.set("page", String(page));
      window.location.assign(url);
      return;
    }
    if (!hasOpenParameter()) setOpenURL(page);
    setOpen(true);
    restorePosition(state, page);
  }

  document.querySelectorAll("[data-open-financial-timeline]").forEach((link) => {
    link.addEventListener("click", (event) => {
      if (new URL(link.href, window.location.href).pathname !== "/assets") return;
      openTimeline(event);
    });
  });

  dialog.querySelectorAll("[data-minimize-financial-timeline]").forEach((button) => {
    button.addEventListener("click", minimizeTimeline);
  });
  dialog.querySelectorAll("[data-close-financial-timeline]").forEach((button) => {
    button.addEventListener("click", closeTimeline);
  });

  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) {
      closeTimeline();
      return;
    }
    const record = event.target.closest("[data-financial-record-key]");
    if (!record || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    const key = record.dataset.financialRecordKey;
    saveState(key);
    try { sessionStorage.setItem("fmlyRecordFocus", key); } catch { /* the row anchor remains available */ }
    setOpen(false, false);
    window.setTimeout(() => window.location.assign(record.href), 120);
  });

  document.addEventListener("keydown", (event) => {
    if (dialog.hidden) return;
    if (event.key === "Escape") {
      event.preventDefault();
      closeTimeline();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = [...dialog.querySelectorAll('a[href],button:not([disabled]),[tabindex]:not([tabindex="-1"])')];
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  });

  window.addEventListener("popstate", () => {
    const open = hasOpenParameter();
    if (!open && !dialog.hidden) saveState();
    setOpen(open, !open);
    if (open) restorePosition(loadState(), Number(dialog.dataset.page) || 1);
  });

  if (hasOpenParameter()) {
    const url = new URL(window.location.href);
    const state = loadState();
    const renderedPage = Number(dialog.dataset.page) || 1;
    if (!url.searchParams.has("page") && state && state.page !== renderedPage) {
      url.searchParams.set("page", String(state.page));
      window.location.replace(url);
      return;
    }
    setOpen(true, false);
    restorePosition(state, renderedPage);
  }
})();
