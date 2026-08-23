(() => {
  const support = typeof window.PublicKeyCredential === 'function' && navigator.credentials;

  function b64urlToBuffer(value) {
    const normalized = String(value).replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4);
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
    return bytes.buffer;
  }

  function bufferToB64url(value) {
    const bytes = new Uint8Array(value);
    let binary = '';
    for (let i = 0; i < bytes.length; i += 1) binary += String.fromCharCode(bytes[i]);
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
  }

  function decodeCreationOptions(publicKey) {
    if (typeof PublicKeyCredential.parseCreationOptionsFromJSON === 'function') {
      return PublicKeyCredential.parseCreationOptionsFromJSON(publicKey);
    }
    const copy = JSON.parse(JSON.stringify(publicKey));
    copy.challenge = b64urlToBuffer(copy.challenge);
    copy.user.id = b64urlToBuffer(copy.user.id);
    (copy.excludeCredentials || []).forEach((item) => { item.id = b64urlToBuffer(item.id); });
    return copy;
  }

  function decodeRequestOptions(publicKey) {
    const copy = JSON.parse(JSON.stringify(publicKey));
    (copy.allowCredentials || []).forEach((item) => { delete item.transports; });
    if (typeof PublicKeyCredential.parseRequestOptionsFromJSON === 'function') {
      return PublicKeyCredential.parseRequestOptionsFromJSON(copy);
    }
    copy.challenge = b64urlToBuffer(copy.challenge);
    (copy.allowCredentials || []).forEach((item) => { item.id = b64urlToBuffer(item.id); });
    return copy;
  }

  function creationCredentialJSON(credential) {
    const response = credential.response;
    const body = {
      id: credential.id,
      rawId: bufferToB64url(credential.rawId),
      type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
      clientExtensionResults: credential.getClientExtensionResults ? credential.getClientExtensionResults() : {},
      response: {
        clientDataJSON: bufferToB64url(response.clientDataJSON),
        attestationObject: bufferToB64url(response.attestationObject),
        transports: response.getTransports ? response.getTransports() : undefined
      }
    };
    if (response.getAuthenticatorData) body.response.authenticatorData = bufferToB64url(response.getAuthenticatorData());
    if (response.getPublicKey) {
      const key = response.getPublicKey();
      if (key) body.response.publicKey = bufferToB64url(key);
    }
    if (response.getPublicKeyAlgorithm) body.response.publicKeyAlgorithm = response.getPublicKeyAlgorithm();
    return body;
  }

  function assertionCredentialJSON(credential) {
    const response = credential.response;
    const body = {
      id: credential.id,
      rawId: bufferToB64url(credential.rawId),
      type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
      clientExtensionResults: credential.getClientExtensionResults ? credential.getClientExtensionResults() : {},
      response: {
        clientDataJSON: bufferToB64url(response.clientDataJSON),
        authenticatorData: bufferToB64url(response.authenticatorData),
        signature: bufferToB64url(response.signature)
      }
    };
    if (response.userHandle) body.response.userHandle = bufferToB64url(response.userHandle);
    return body;
  }

  async function jsonFetch(url, options = {}) {
    const response = await fetch(url, {
      credentials: 'same-origin',
      cache: 'no-store',
      ...options,
      headers: { Accept: 'application/json', ...(options.headers || {}) }
    });
    const contentType = response.headers.get('content-type') || '';
    const payload = contentType.includes('application/json') ? await response.json() : { message: (await response.text()).trim() };
    if (!response.ok || payload.ok === false) throw new Error(payload.message || `请求失败（HTTP ${response.status}）`);
    return payload;
  }

  function friendlyError(error) {
    if (!error) return 'Passkey 操作失败。';
    if (error.name === 'NotAllowedError') return 'Passkey 操作已取消、超时，或当前设备没有直接可用的凭据。若凭据在另一台设备，请在系统 Passkey 面板选择“使用其他设备 / 手机或平板电脑”，让本机显示 FIDO 二维码后扫码验证。';
    if (error.name === 'InvalidStateError') return '当前设备可能已经为这个登录身份创建过同一 Passkey。';
    if (error.name === 'SecurityError') return '当前地址不满足 Passkey 的安全域要求，请使用 HTTPS 域名访问。';
    if (error.name === 'NotSupportedError') return '当前浏览器或设备不支持所需的 Passkey 能力。';
    return error.message || String(error);
  }

  function setStatus(node, message, isError = false) {
    if (!node) return;
    node.hidden = !message;
    node.textContent = message || '';
    node.classList.toggle('is-error', isError);
    node.classList.toggle('is-success', !isError && Boolean(message));
  }

  function enablePasskeyForm(button, status) {
    if (!button) return false;
    if (!support || !window.isSecureContext) {
      button.disabled = true;
      button.hidden = true;
      setStatus(status, !support ? '当前浏览器不支持 Passkey，请使用微信扫码登录。' : '当前页面不是 HTTPS 安全上下文，Passkey 不可用，请使用微信扫码登录。', true);
      return false;
    }
    button.hidden = false;
    button.disabled = false;
    return true;
  }

  function setupUnifiedIdentity() {
    const form = document.querySelector('[data-passkey-identity]');
    if (!form) return;
    const button = form.querySelector('[data-passkey-identity-button]');
    const status = form.querySelector('[data-passkey-identity-status]');
    const remarkWrap = form.querySelector('[data-passkey-remark-wrap]');
    const phoneInput = form.elements.phone;
    const remarkInput = form.elements.remark;
    if (!enablePasskeyForm(button, status)) return;

    let createPhone = '';

    phoneInput.addEventListener('input', () => {
      if (phoneInput.value.trim() === createPhone) return;
      createPhone = '';
      remarkWrap.hidden = true;
      remarkInput.required = false;
      remarkInput.value = '';
      setStatus(status, '');
    });

    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      if (!form.reportValidity()) return;
      const phone = phoneInput.value.trim();
      button.disabled = true;
      try {
        if (createPhone !== phone) {
          setStatus(status, '正在确认该手机号是否已经有 Passkey 身份…');
          const resolved = await jsonFetch('/auth/passkey/identity/resolve', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ phone })
          });
          if (!resolved.exists) {
            createPhone = phone;
            remarkWrap.hidden = false;
            remarkInput.required = true;
            setStatus(status, '这是首次使用该手机号。请补充身份备注，然后再次点击“使用Passkey身份”创建新的登录身份。');
            remarkInput.focus();
            button.disabled = false;
            return;
          }

          setStatus(status, '已找到原登录身份。正在调用系统 Passkey 面板；本机没有凭据时请选择“使用其他设备”。');
          const options = await jsonFetch('/auth/passkey/login/options', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ phone })
          });
          const credential = await navigator.credentials.get({ publicKey: decodeRequestOptions(options.publicKey) });
          if (!credential) throw new Error('没有取得 Passkey 凭据');
          setStatus(status, 'Passkey 已验证，正在恢复原来的 FmlySys 登录身份…');
          const result = await jsonFetch('/auth/passkey/login/finish', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(assertionCredentialJSON(credential))
          });
          window.location.assign(result.redirect || '/');
          return;
        }

        const remark = remarkInput.value.trim();
        if (!remark) {
          remarkInput.focus();
          button.disabled = false;
          return;
        }
        setStatus(status, '正在创建新的 Passkey 登录身份…');
        const options = await jsonFetch('/auth/passkey/create/options', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ phone, remark })
        });
        const credential = await navigator.credentials.create({ publicKey: decodeCreationOptions(options.publicKey) });
        if (!credential) throw new Error('没有取得新 Passkey 凭据');
        setStatus(status, '正在保存新的登录身份…');
        const result = await jsonFetch('/auth/passkey/create/finish', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(creationCredentialJSON(credential))
        });
        window.location.assign(result.redirect || '/');
      } catch (error) {
        setStatus(status, friendlyError(error), true);
        button.disabled = false;
      }
    });
  }

  function setupAdd() {
    const form = document.querySelector('[data-passkey-add]');
    if (!form) return;
    const button = form.querySelector('[data-passkey-add-button]');
    const status = form.querySelector('[data-passkey-add-status]');
    if (!enablePasskeyForm(button, status)) return;

    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      if (!form.reportValidity()) return;
      const remark = form.elements.remark.value.trim();
      button.disabled = true;
      setStatus(status, '正在为当前设备准备新的 Passkey…');
      try {
        const options = await jsonFetch('/passkey/account/register/options', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ remark })
        });
        const credential = await navigator.credentials.create({ publicKey: decodeCreationOptions(options.publicKey) });
        if (!credential) throw new Error('没有取得新 Passkey 凭据');
        setStatus(status, '正在把当前设备 Passkey 加入同一个登录身份…');
        const result = await jsonFetch('/passkey/account/register/finish', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(creationCredentialJSON(credential))
        });
        window.location.assign(result.redirect || '/passkey/account');
      } catch (error) {
        setStatus(status, friendlyError(error), true);
        button.disabled = false;
      }
    });
  }

  setupUnifiedIdentity();
  setupAdd();
})();
