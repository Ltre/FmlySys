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

  function monthStateLabel(day) {
    switch (day.state) {
      case 'complete':
        return '全部完成';
      case 'partial':
        return '部分完成';
      case 'missed':
        return '全部未完成';
      case 'open':
        return day.date === currentMonthOverview?.today ? '今天，暂不结算' : '未来日期，暂不结算';
      default:
        return '当天无服药计划';
    }
  }

  let currentMonthOverview = null;

  function createMonthDay(day) {
    const cell = document.createElement('div');
    cell.className = `medication-month-day medication-month-${day.state}`;
    if (currentMonthOverview && day.date === currentMonthOverview.today) {
      cell.classList.add('medication-month-today');
    }

    const number = document.createElement('strong');
    number.className = 'medication-month-day-number';
    number.textContent = String(day.day);
    cell.appendChild(number);

    if (day.scheduled > 0) {
      const progress = document.createElement('span');
      progress.className = 'medication-month-progress';
      progress.textContent = `${day.taken}/${day.scheduled}`;
      cell.appendChild(progress);
    }

    const label = monthStateLabel(day);
    cell.title = `${day.date}：${label}${day.scheduled > 0 ? `（已完成 ${day.taken}/${day.scheduled}）` : ''}`;
    cell.setAttribute('aria-label', cell.title);
    return cell;
  }

  function renderMonthOverview(data, anchor) {
    currentMonthOverview = data;
    const existing = document.querySelector('[data-medication-month-overview]');
    if (existing) {
      existing.remove();
    }

    const section = document.createElement('section');
    section.className = 'card medication-month-overview';
    section.dataset.medicationMonthOverview = '1';

    const head = document.createElement('div');
    head.className = 'section-head medication-month-head';
    const titleWrap = document.createElement('div');
    const title = document.createElement('h2');
    const [year, month] = data.month.split('-');
    title.textContent = `${year} 年 ${Number(month)} 月服用情况`;
    const note = document.createElement('p');
    note.className = 'muted';
    note.textContent = '独立于下方统计周期。今天和未来日期保持白色，过完当天后再按完成情况着色。';
    titleWrap.append(title, note);
    head.appendChild(titleWrap);
    section.appendChild(head);

    const legend = document.createElement('div');
    legend.className = 'medication-month-legend';
    [
      ['complete', '全部完成'],
      ['partial', '部分完成'],
      ['missed', '全部未完成'],
      ['none', '无计划 / 今天 / 未来'],
    ].forEach(([state, text]) => {
      const item = document.createElement('span');
      const swatch = document.createElement('i');
      swatch.className = `medication-month-swatch medication-month-${state}`;
      item.append(swatch, document.createTextNode(text));
      legend.appendChild(item);
    });
    section.appendChild(legend);

    const grid = document.createElement('div');
    grid.className = 'medication-month-grid';
    ['一', '二', '三', '四', '五', '六', '日'].forEach((weekday) => {
      const header = document.createElement('div');
      header.className = 'medication-month-weekday';
      header.textContent = weekday;
      grid.appendChild(header);
    });
    for (let i = 0; i < data.weekday_offset; i += 1) {
      const blank = document.createElement('div');
      blank.className = 'medication-month-blank';
      grid.appendChild(blank);
    }
    data.days.forEach((day) => grid.appendChild(createMonthDay(day)));
    section.appendChild(grid);

    anchor.before(section);
  }

  async function loadMonthOverview() {
    const form = document.getElementById('medication-filter-form');
    const toolbar = document.querySelector('.medication-toolbar');
    if (!form || !toolbar) {
      return;
    }
    const member = form.querySelector('input[name="member"]')?.value;
    if (!member) {
      return;
    }

    try {
      const response = await fetch(`/medication/month-overview?member=${encodeURIComponent(member)}`, {
        credentials: 'same-origin',
        headers: {'Accept': 'application/json'},
      });
      if (!response.ok) {
        throw new Error(await response.text() || '本月服用情况加载失败');
      }
      renderMonthOverview(await response.json(), toolbar);
    } catch (err) {
      console.error('medication month overview:', err);
    }
  }

  loadMonthOverview();
})();
