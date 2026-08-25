(() => {
  const button = document.querySelector('[data-enable-medication-push]');

  const decodeKey = (value) => {
    const pad = '='.repeat((4 - value.length % 4) % 4);
    const raw = atob((value + pad).replace(/-/g, '+').replace(/_/g, '/'));
    return Uint8Array.from(raw, (char) => char.charCodeAt(0));
  };

  async function enablePush() {
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
      throw new Error('当前浏览器不支持 PWA Push');
    }

    const registration = await navigator.serviceWorker.register('/medication-sw.js', {
      scope: '/',
    });
    const permission = await Notification.requestPermission();
    if (permission !== 'granted') {
      throw new Error('通知权限未允许');
    }

    const keyResponse = await fetch('/medication/push/public-key', {
      credentials: 'same-origin',
    });
    if (!keyResponse.ok) {
      throw new Error('无法取得 PWA 推送公钥');
    }

    const {public_key: publicKey} = await keyResponse.json();
    let subscription = await registration.pushManager.getSubscription();
    if (!subscription) {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: decodeKey(publicKey),
      });
    }

    const saveResponse = await fetch('/medication/push/subscribe', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(subscription),
    });
    if (!saveResponse.ok) {
      throw new Error(await saveResponse.text() || '保存 PWA 订阅失败');
    }

    return 'PWA 服药通知已启用';
  }

  if (button) {
    button.addEventListener('click', async () => {
      button.disabled = true;
      const originalText = button.textContent;
      button.textContent = '正在启用…';

      try {
        alert(await enablePush());
      } catch (err) {
        alert(`启用失败：${err.message || err}`);
      } finally {
        button.disabled = false;
        button.textContent = originalText;
      }
    });
  }

  navigator.serviceWorker?.addEventListener('message', (event) => {
    if (event.data?.type !== 'fmlysys-medication-reminder' || !event.data.voice) {
      return;
    }
    if ('speechSynthesis' in window) {
      speechSynthesis.cancel();
      const utterance = new SpeechSynthesisUtterance(event.data.voice);
      utterance.lang = 'zh-CN';
      speechSynthesis.speak(utterance);
    }
  });
})();
