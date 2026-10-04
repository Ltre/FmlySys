(() => {
  const rows = document.getElementById('wechat-event-rows');
  const status = document.getElementById('wechat-events-status');
  if (!rows || !status) return;

  const addCell = (row, value, className = '') => {
    const cell = document.createElement('td');
    if (className) cell.className = className;
    if (value) {
      const code = document.createElement('code');
      code.textContent = value;
      cell.append(code);
    } else {
      cell.textContent = '—';
    }
    row.append(cell);
  };

  const render = (events) => {
    rows.replaceChildren();
    if (!events.length) {
      const row = document.createElement('tr');
      const cell = document.createElement('td');
      cell.colSpan = 5;
      cell.className = 'muted';
      cell.textContent = '还没有收到已验证的事件。现在可以点击公众号中的测试菜单。';
      row.append(cell);
      rows.append(row);
      return;
    }

    events.forEach((event) => {
      const row = document.createElement('tr');
      const received = new Date(event.received_at);
      addCell(row, Number.isNaN(received.getTime()) ? event.received_at : received.toLocaleString());
      const eventCell = document.createElement('td');
      const badge = document.createElement('span');
      badge.className = `tag ${event.event === 'VIEW' ? 'dev-event-view' : 'dev-event-other'}`;
      badge.textContent = event.event || event.msg_type || '未知';
      eventCell.append(badge);
      row.append(eventCell);
      addCell(row, event.from_user_name);
      addCell(row, event.event_key);
      addCell(row, event.msg_type);
      rows.append(row);
    });
  };

  const refresh = async () => {
    try {
      const response = await fetch('/admin/api/wechat-callback-events', { cache: 'no-store', credentials: 'same-origin' });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const result = await response.json();
      render(Array.isArray(result.events) ? result.events : []);
      status.textContent = result.configured ? '回调监测中' : '配置未完整';
      status.className = `tag ${result.configured ? 'dev-status-ready' : 'dev-status-missing'}`;
    } catch (_) {
      status.textContent = '读取失败';
      status.className = 'tag dev-status-missing';
    }
  };

  refresh();
  window.setInterval(() => {
    if (!document.hidden) refresh();
  }, 3000);
})();
