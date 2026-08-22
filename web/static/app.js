(() => {
  const header = document.querySelector('[data-admin-tap]');
  if (!header) return;

  let tapCount = 0;
  let lastTapAt = 0;
  const maxGapMs = 700;

  header.addEventListener('click', (event) => {
    if (typeof event.button === 'number' && event.button !== 0) return;
    if (event.target.closest('a,button,input,select,textarea,label,form,summary')) return;

    const now = performance.now();
    tapCount = now - lastTapAt <= maxGapMs ? tapCount + 1 : 1;
    lastTapAt = now;

    if (tapCount < 7) return;
    tapCount = 0;
    lastTapAt = 0;

    const opened = window.open('/admin', '_blank', 'noopener,noreferrer');
    if (opened) opened.opener = null;
  });
})();
