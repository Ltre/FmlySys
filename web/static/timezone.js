(() => {
  "use strict";

  const defaultTimezone = "Asia/Shanghai";
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || defaultTimezone;
  document.cookie = `fmly_timezone=${encodeURIComponent(timezone)}; Max-Age=31536000; Path=/; SameSite=Lax`;

  for (const input of document.querySelectorAll("[data-device-timezone]")) {
    input.value = timezone;
  }
  for (const node of document.querySelectorAll("[data-current-timezone]")) {
    node.textContent = timezone;
  }

  const formatter = new Intl.DateTimeFormat("zh-CN", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });

  function formatISO(raw) {
    const value = String(raw || "").trim();
    if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)) {
      return null;
    }
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
      return null;
    }
    return formatter.format(parsed);
  }

  for (const node of document.querySelectorAll("[data-fmly-datetime]")) {
    const raw = node.getAttribute("data-fmly-datetime") || node.textContent;
    const formatted = formatISO(raw);
    if (formatted) {
      node.textContent = formatted;
      node.title = `${raw} · ${timezone}`;
    }
  }

  const isoPattern = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})/g;
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  const candidates = [];
  while (walker.nextNode()) {
    const text = walker.currentNode;
    const parent = text.parentElement;
    if (!parent || parent.closest("script,style,textarea,input,code,pre,[data-fmly-datetime]")) {
      continue;
    }
    if (isoPattern.test(text.nodeValue || "")) {
      candidates.push(text);
    }
    isoPattern.lastIndex = 0;
  }

  for (const text of candidates) {
    text.nodeValue = (text.nodeValue || "").replace(isoPattern, (raw) => formatISO(raw) || raw);
  }
})();
