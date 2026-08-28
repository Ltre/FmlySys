(() => {
  const margin = 10;

  document.querySelectorAll('[data-floating-actions]').forEach((root) => {
    const toggle = root.querySelector('.floating-action-toggle');
    const menu = root.querySelector('.floating-action-menu');
    if (!toggle || !menu) return;

    const storageKey = `fmly-floating:${root.dataset.storageKey || 'default'}`;
    let pointerID = null;
    let startX = 0;
    let startY = 0;
    let originLeft = 0;
    let originTop = 0;
    let moved = false;
    let suppressClick = false;

    const clamp = (value, min, max) => Math.min(Math.max(value, min), Math.max(min, max));

    function updateMenuDirection() {
      const rect = root.getBoundingClientRect();
      root.classList.toggle('floating-open-down', rect.top < window.innerHeight * 0.48);
      root.classList.toggle('floating-align-left', rect.left < window.innerWidth * 0.5);
    }

    function setOpen(open) {
      root.classList.toggle('is-open', open);
      toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
      if (open) updateMenuDirection();
    }

    function place(left, top, remember) {
      const rect = root.getBoundingClientRect();
      const maxLeft = window.innerWidth - rect.width - margin;
      const maxTop = window.innerHeight - rect.height - margin;
      const nextLeft = clamp(left, margin, maxLeft);
      const nextTop = clamp(top, margin, maxTop);
      root.style.left = `${nextLeft}px`;
      root.style.top = `${nextTop}px`;
      root.style.right = 'auto';
      root.style.bottom = 'auto';
      updateMenuDirection();
      if (remember) {
        try { localStorage.setItem(storageKey, JSON.stringify({ left: nextLeft, top: nextTop })); } catch { /* storage may be unavailable */ }
      }
    }

    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) || 'null');
      if (saved && Number.isFinite(saved.left) && Number.isFinite(saved.top)) {
        requestAnimationFrame(() => place(saved.left, saved.top, false));
      }
    } catch { /* ignore stale position data */ }

    toggle.addEventListener('pointerdown', (event) => {
      if (event.button !== 0) return;
      pointerID = event.pointerId;
      startX = event.clientX;
      startY = event.clientY;
      const rect = root.getBoundingClientRect();
      originLeft = rect.left;
      originTop = rect.top;
      moved = false;
      toggle.setPointerCapture(pointerID);
      root.classList.add('is-dragging');
    });

    toggle.addEventListener('pointermove', (event) => {
      if (event.pointerId !== pointerID) return;
      const dx = event.clientX - startX;
      const dy = event.clientY - startY;
      if (!moved && Math.hypot(dx, dy) < 6) return;
      moved = true;
      event.preventDefault();
      place(originLeft + dx, originTop + dy, false);
    });

    const finishDrag = (event) => {
      if (event.pointerId !== pointerID) return;
      if (toggle.hasPointerCapture(pointerID)) toggle.releasePointerCapture(pointerID);
      pointerID = null;
      root.classList.remove('is-dragging');
      if (moved) {
        const rect = root.getBoundingClientRect();
        place(rect.left, rect.top, true);
        suppressClick = true;
      }
    };
    toggle.addEventListener('pointerup', finishDrag);
    toggle.addEventListener('pointercancel', finishDrag);

    toggle.addEventListener('click', () => {
      if (suppressClick) {
        suppressClick = false;
        return;
      }
      setOpen(!root.classList.contains('is-open'));
    });

    menu.addEventListener('click', (event) => {
      const link = event.target.closest('a');
      if (!link) return;
      const target = link.hash ? document.querySelector(link.hash) : null;
      if (target instanceof HTMLDetailsElement) target.open = true;
      setOpen(false);
    });
    document.addEventListener('click', (event) => {
      if (!root.contains(event.target)) setOpen(false);
    });
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') setOpen(false);
    });
    window.addEventListener('resize', () => {
      const rect = root.getBoundingClientRect();
      if (root.style.left) place(rect.left, rect.top, true);
      else updateMenuDirection();
    });
  });
})();
