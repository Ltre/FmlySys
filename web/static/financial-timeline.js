(() => {
  "use strict";

  const dialog = document.getElementById("financial-timeline-modal");
  if (!dialog) return;

  let returnFocus = null;

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
    } else if (!open && returnFocus && returnFocus.isConnected) {
      returnFocus.focus();
    }
  }

  function updateOpenURL() {
    const url = new URL(window.location.href);
    url.searchParams.set("timeline", "1");
    if (!url.searchParams.has("page")) url.searchParams.set("page", "1");
    window.history.pushState({}, "", url);
  }

  function closeTimeline() {
    const url = new URL(window.location.href);
    url.searchParams.delete("timeline");
    url.searchParams.delete("page");
    window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
    setOpen(false);
  }

  document.querySelectorAll("[data-open-financial-timeline]").forEach((link) => {
    link.addEventListener("click", (event) => {
      if (new URL(link.href, window.location.href).pathname !== "/assets") return;
      event.preventDefault();
      if (!hasOpenParameter()) updateOpenURL();
      setOpen(true);
    });
  });

  dialog.querySelectorAll("[data-close-financial-timeline]").forEach((button) => {
    button.addEventListener("click", closeTimeline);
  });
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) closeTimeline();
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
  window.addEventListener("popstate", () => setOpen(hasOpenParameter(), false));

  if (hasOpenParameter()) setOpen(true, false);
})();
