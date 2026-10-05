(function () {
  const root = document.querySelector('[data-totp-auth]');
  if (!root) return;

  const tabs = Array.from(root.querySelectorAll('[data-totp-tab]'));
  const panes = Array.from(root.querySelectorAll('[data-totp-pane]'));
  const qr = root.querySelector('[data-totp-qr]');
  const qrLabel = root.querySelector('[data-totp-qr-label]');
  const qrStatus = root.querySelector('[data-totp-qr-status]');
  const registerButton = root.querySelector('[data-totp-register-button]');
  const username = root.querySelector('[data-totp-username]');
  const remark = root.querySelector('[data-totp-remark]');
  const form = root.querySelector('[data-totp-register-form]');
  if (!qr || !qrLabel || !qrStatus || !registerButton || !username || !remark || !form) return;

  let timer;
  let ready = false;
  let requestSequence = 0;
  let requestController;
  let qrObjectURL = '';

  function updateQRCodeLabel() {
    const accountName = username.value.trim();
    const note = remark.value.trim();
    qrLabel.textContent = accountName
      ? '二维码标签：Fmly: ' + accountName + (note ? ' [' + note + ']' : '')
      : '请先输入用户名以生成二维码';
  }

  function qrURL() {
    const params = new URLSearchParams({ username: username.value, remark: remark.value });
    return '/auth/2fa/register/qr?' + params.toString();
  }

  function hideCurrentQRCode() {
    qr.hidden = true;
    qr.removeAttribute('src');
    if (qrObjectURL) {
      URL.revokeObjectURL(qrObjectURL);
      qrObjectURL = '';
    }
  }

  function invalidateQRCode() {
    requestSequence++;
    if (requestController) {
      requestController.abort();
      requestController = null;
    }
    ready = false;
    registerButton.disabled = true;
    hideCurrentQRCode();
  }

  async function refreshQRCode() {
    invalidateQRCode();
    const accountName = username.value.trim();
    updateQRCodeLabel();
    if (!accountName) {
      qrStatus.textContent = '输入用户名后生成绑定二维码。';
      return;
    }

    const requestID = ++requestSequence;
    const controller = new AbortController();
    requestController = controller;
    qrStatus.textContent = '正在生成绑定二维码…';
    try {
      const response = await fetch(qrURL(), {
        cache: 'no-store',
        credentials: 'same-origin',
        signal: controller.signal
      });
      if (!response.ok) throw new Error('二维码请求失败');

      const encodedLabel = response.headers.get('X-Fmly-TOTP-Label');
      const png = await response.blob();
      if (requestID !== requestSequence) return;
      if (encodedLabel) qrLabel.textContent = '二维码标签：' + decodeURIComponent(encodedLabel.replace(/\+/g, ' '));

      const nextObjectURL = URL.createObjectURL(png);
      qrObjectURL = nextObjectURL;
      qr.onload = () => {
        if (requestID !== requestSequence) return;
        ready = true;
        registerButton.disabled = false;
        qrStatus.textContent = '二维码已就绪。请核对上方标签后扫码。';
      };
      qr.onerror = () => {
        if (requestID !== requestSequence) return;
        ready = false;
        registerButton.disabled = true;
        qrStatus.textContent = '二维码图片无法显示，请修改信息后重试。';
      };
      qr.src = nextObjectURL;
      qr.hidden = false;
    } catch (error) {
      if (requestID !== requestSequence || error.name === 'AbortError') return;
      qrStatus.textContent = '二维码生成失败，请确认用户名格式后重试。';
    } finally {
      if (requestID === requestSequence) requestController = null;
    }
  }

  function activateTab(name) {
    tabs.forEach((tab) => {
      const active = tab.dataset.totpTab === name;
      tab.setAttribute('aria-selected', active ? 'true' : 'false');
      tab.tabIndex = active ? 0 : -1;
    });
    panes.forEach((pane) => {
      pane.hidden = pane.dataset.totpPane !== name;
    });
    if (name === 'register') refreshQRCode();
  }

  function scheduleRefresh() {
    clearTimeout(timer);
    invalidateQRCode();
    updateQRCodeLabel();
    qrStatus.textContent = username.value.trim()
      ? '正在更新二维码标签…'
      : '输入用户名后生成绑定二维码。';
    timer = setTimeout(refreshQRCode, 180);
  }

  username.addEventListener('input', scheduleRefresh);
  remark.addEventListener('input', scheduleRefresh);
  form.addEventListener('submit', (event) => {
    if (!ready) {
      event.preventDefault();
      qrStatus.textContent = '请等待与当前用户名和备注对应的二维码生成后再注册。';
    }
  });

  tabs.forEach((tab, index) => {
    tab.addEventListener('click', () => activateTab(tab.dataset.totpTab));
    tab.addEventListener('keydown', (event) => {
      if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
      event.preventDefault();
      const offset = event.key === 'ArrowRight' ? 1 : -1;
      const next = tabs[(index + offset + tabs.length) % tabs.length];
      activateTab(next.dataset.totpTab);
      next.focus();
    });
  });

  updateQRCodeLabel();
  activateTab(root.dataset.defaultTab === 'register' ? 'register' : 'login');
})();
