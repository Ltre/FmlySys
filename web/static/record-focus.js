(() => {
  const recordHeader = 'X-Fmly-Record-Key';
  const originalFetch = window.fetch.bind(window);

  window.fetch = async (...args) => {
    const response = await originalFetch(...args);
    const key = response.headers.get(recordHeader);
    if (key) sessionStorage.setItem('fmlyRecordFocus', key);
    return response;
  };

  const style = document.createElement('style');
  style.textContent = `
    .record-focus-highlight {
      outline: 3px solid #1a73e8 !important;
      outline-offset: -1px;
      background: #e8f0fe !important;
      box-shadow: 0 0 0 4px rgba(26,115,232,.16);
      transition: outline-color .35s ease, background-color .7s ease, box-shadow .7s ease;
    }
  `;
  document.head.appendChild(style);

  function sectionByHeading(text) {
    return Array.from(document.querySelectorAll('section.card')).find((section) => {
      const h = section.querySelector('h2');
      return h && h.textContent.trim() === text;
    }) || null;
  }

  function targetSection(name) {
    const direct = document.getElementById(name);
    if (direct) return direct;
    const headings = {
      'asset-movements': '资产变动流水',
      'expense-records': '消费流水',
      'transfer-records': '内部转账流水',
      'reimbursement-records': '报销流水'
    };
    return headings[name] ? sectionByHeading(headings[name]) : null;
  }

  function normalize(value) {
    return String(value || '').replace(/\s+/g, ' ').trim();
  }

  function highlight(row) {
    if (!row) return false;
    row.classList.add('record-focus-highlight');
    row.tabIndex = -1;
    row.scrollIntoView({ behavior: 'smooth', block: 'center' });
    window.setTimeout(() => {
      try { row.focus({ preventScroll: true }); } catch { row.focus(); }
    }, 280);
    window.setTimeout(() => row.classList.remove('record-focus-highlight'), 4200);
    return true;
  }

  async function consumeRecordFocus() {
    const key = sessionStorage.getItem('fmlyRecordFocus');
    if (!key) return;
    const match = key.match(/^(asset_event|expense|transfer|reimbursement):(\d+)$/);
    if (!match) {
      sessionStorage.removeItem('fmlyRecordFocus');
      return;
    }
    try {
      const response = await originalFetch(`/api/money-record/${match[1]}/${match[2]}`, {
        credentials: 'same-origin',
        cache: 'no-store',
        headers: { Accept: 'application/json' }
      });
      if (!response.ok) return;
      const info = await response.json();
      let row = document.querySelector(`[data-record-key="${CSS.escape(key)}"]`);
      if (!row && info.href) {
        const link = Array.from(document.querySelectorAll('a[href]')).find((candidate) => candidate.getAttribute('href') === info.href);
        row = link?.closest('tr') || null;
      }
      if (!row) {
        const section = targetSection(info.section);
        const tokens = (info.tokens || []).map(normalize).filter(Boolean);
        if (section) {
          row = Array.from(section.querySelectorAll('tr')).find((candidate) => {
            if (candidate.querySelector('th')) return false;
            const text = normalize(candidate.textContent);
            return tokens.every((token) => text.includes(token));
          }) || null;
        }
      }
      if (highlight(row)) sessionStorage.removeItem('fmlyRecordFocus');
    } catch (error) {
      console.warn('FmlySys record focus failed', error);
    }
  }

  consumeRecordFocus();
})();
