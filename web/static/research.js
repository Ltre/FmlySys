(() => {
  const root = document.querySelector('[data-research-member]');
  if (!root) return;

  const memberID = root.dataset.researchMember;
  const storageKey = `fmly-research-tabs-v1:${memberID}`;
  const MAX_TABS = 5;
  const tabsEl = root.querySelector('[data-research-tabs]');
  const newTabButton = root.querySelector('[data-research-new-tab]');
  const form = root.querySelector('[data-research-address-form]');
  const address = root.querySelector('[data-research-address]');
  const frame = root.querySelector('[data-research-frame]');
  const start = root.querySelector('[data-research-start]');
  const backButton = root.querySelector('[data-research-back]');
  const forwardButton = root.querySelector('[data-research-forward]');
  const refreshButton = root.querySelector('[data-research-refresh]');

  let sequence = 0;
  let state = loadState();

  function newID() {
    sequence += 1;
    return `${Date.now().toString(36)}-${sequence.toString(36)}`;
  }

  function blankTab() {
    return {id: newID(), title: '新标签页', url: '', history: [], historyIndex: -1};
  }

  function loadState() {
    try {
      const parsed = JSON.parse(localStorage.getItem(storageKey) || 'null');
      if (parsed && Array.isArray(parsed.tabs) && parsed.tabs.length > 0) {
        parsed.tabs = parsed.tabs.slice(0, MAX_TABS).map((tab) => ({
          id: String(tab.id || newID()),
          title: String(tab.title || '标签页').slice(0, 80),
          url: String(tab.url || ''),
          history: Array.isArray(tab.history) ? tab.history.slice(-30).map(String) : [],
          historyIndex: Number.isInteger(tab.historyIndex) ? tab.historyIndex : -1,
        }));
        if (!parsed.tabs.some((tab) => tab.id === parsed.activeID)) parsed.activeID = parsed.tabs[0].id;
        return parsed;
      }
    } catch (error) {
      console.warn('research tab state:', error);
    }
    const tab = blankTab();
    return {tabs: [tab], activeID: tab.id};
  }

  function saveState() {
    localStorage.setItem(storageKey, JSON.stringify(state));
  }

  function activeTab() {
    return state.tabs.find((tab) => tab.id === state.activeID) || state.tabs[0];
  }

  function normalizeURL(raw) {
    raw = String(raw || '').trim();
    if (!raw) return '';
    if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(raw)) raw = `https://${raw}`;
    try {
      const parsed = new URL(raw);
      if (!['http:', 'https:'].includes(parsed.protocol)) return '';
      return parsed.href;
    } catch (_) {
      return '';
    }
  }

  function proxyURL(url) {
    return `/research/proxy?member=${encodeURIComponent(memberID)}&url=${encodeURIComponent(url)}`;
  }

  function renderTabs() {
    tabsEl.querySelectorAll('[data-research-tab]').forEach((node) => node.remove());
    state.tabs.forEach((tab) => {
      const wrap = document.createElement('div');
      wrap.className = `research-tab${tab.id === state.activeID ? ' active' : ''}`;
      wrap.dataset.researchTab = tab.id;

      const select = document.createElement('button');
      select.type = 'button';
      select.className = 'research-tab-select';
      select.textContent = tab.title || '标签页';
      select.title = tab.url || '新标签页';
      select.addEventListener('click', () => switchTab(tab.id));

      const close = document.createElement('button');
      close.type = 'button';
      close.className = 'research-tab-close';
      close.textContent = '×';
      close.title = '关闭标签页';
      close.addEventListener('click', () => closeTab(tab.id));
      wrap.append(select, close);
      tabsEl.insertBefore(wrap, newTabButton);
    });
    newTabButton.disabled = state.tabs.length >= MAX_TABS;
  }

  function renderActive() {
    const tab = activeTab();
    renderTabs();
    address.value = tab.url || '';
    backButton.disabled = tab.historyIndex <= 0;
    forwardButton.disabled = tab.historyIndex < 0 || tab.historyIndex >= tab.history.length - 1;
    if (!tab.url) {
      frame.removeAttribute('src');
      frame.hidden = true;
      start.hidden = false;
      return;
    }
    start.hidden = true;
    frame.hidden = false;
    if (frame.dataset.currentURL !== tab.url) {
      frame.dataset.currentURL = tab.url;
      frame.src = proxyURL(tab.url);
    }
  }

  function navigate(raw, {replaceHistory = false} = {}) {
    const url = normalizeURL(raw);
    if (!url) {
      alert('请输入有效的 http/https 网址');
      return;
    }
    const tab = activeTab();
    tab.url = url;
    tab.title = new URL(url).hostname;
    if (!replaceHistory) {
      tab.history = tab.history.slice(0, tab.historyIndex + 1);
      tab.history.push(url);
      if (tab.history.length > 30) tab.history.shift();
      tab.historyIndex = tab.history.length - 1;
    }
    saveState();
    frame.dataset.currentURL = '';
    renderActive();
  }

  function switchTab(id) {
    if (!state.tabs.some((tab) => tab.id === id)) return;
    state.activeID = id;
    saveState();
    frame.dataset.currentURL = '';
    renderActive();
  }

  function addTab(url = '') {
    if (state.tabs.length >= MAX_TABS) {
      alert('最多只能同时打开 5 个标签页。请先关闭一个标签页。');
      return;
    }
    const tab = blankTab();
    state.tabs.push(tab);
    state.activeID = tab.id;
    saveState();
    frame.dataset.currentURL = '';
    renderActive();
    if (url) navigate(url);
    else address.focus();
  }

  function closeTab(id) {
    const index = state.tabs.findIndex((tab) => tab.id === id);
    if (index < 0) return;
    state.tabs.splice(index, 1);
    if (state.tabs.length === 0) state.tabs.push(blankTab());
    if (state.activeID === id) state.activeID = state.tabs[Math.min(index, state.tabs.length - 1)].id;
    saveState();
    frame.dataset.currentURL = '';
    renderActive();
  }

  function historyStep(delta) {
    const tab = activeTab();
    const next = tab.historyIndex + delta;
    if (next < 0 || next >= tab.history.length) return;
    tab.historyIndex = next;
    tab.url = tab.history[next];
    saveState();
    frame.dataset.currentURL = '';
    renderActive();
  }

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    navigate(address.value);
  });
  newTabButton.addEventListener('click', () => addTab());
  backButton.addEventListener('click', () => historyStep(-1));
  forwardButton.addEventListener('click', () => historyStep(1));
  refreshButton.addEventListener('click', () => {
    const tab = activeTab();
    if (!tab.url) return;
    frame.dataset.currentURL = '';
    renderActive();
  });

  root.querySelectorAll('[data-research-open]').forEach((button) => {
    button.addEventListener('click', () => navigate(button.dataset.researchOpen));
  });

  frame.addEventListener('load', () => {
    try {
      const location = new URL(frame.contentWindow.location.href);
      if (location.pathname !== '/research/proxy') return;
      const target = location.searchParams.get('url');
      const url = normalizeURL(target);
      if (!url) return;
      const tab = activeTab();
      if (tab.url !== url) {
        tab.url = url;
        tab.history = tab.history.slice(0, tab.historyIndex + 1);
        tab.history.push(url);
        if (tab.history.length > 30) tab.history.shift();
        tab.historyIndex = tab.history.length - 1;
      }
      const pageTitle = frame.contentDocument?.title?.trim();
      tab.title = (pageTitle || new URL(url).hostname).slice(0, 80);
      address.value = url;
      saveState();
      renderTabs();
    } catch (_) {
      // sandbox/浏览器策略不允许读取时，保留地址栏已有状态即可。
    }
  });

  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/research/sw.js', {scope: '/research/'}).catch((error) => {
      console.warn('research cache service worker:', error);
    });
  }

  renderActive();
})();
