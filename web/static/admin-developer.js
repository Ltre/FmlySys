(() => {
  const body = document.getElementById('diagnostic-entries');
  const updated = document.getElementById('diagnostic-updated');
  const refresh = document.getElementById('refresh-diagnostics');
  if (!body || !updated) return;

  const appendCell = (row, value, className) => {
    const cell = document.createElement('td');
    if (className) cell.className = className;
    cell.textContent = value || '—';
    row.append(cell);
    return cell;
  };

  const formatTime = (value) => {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  };

  function render(entries) {
    body.replaceChildren();
    if (!entries.length) {
      const row = document.createElement('tr');
      const cell = appendCell(row, '暂时没有记录。点击上方手机探测链接后，记录会自动出现。', 'empty-state');
      cell.colSpan = 6;
      body.append(row);
      return;
    }

    for (const entry of entries) {
      const row = document.createElement('tr');
      appendCell(row, formatTime(entry.at));

      const resultCell = document.createElement('td');
      const kind = document.createElement('span');
      kind.className = `diag-kind ${entry.kind === 'error' ? 'diag-kind-error' : 'diag-kind-request'}`;
      kind.textContent = entry.kind === 'error' ? '错误' : '请求';
      resultCell.append(kind, document.createTextNode(' '));
      const status = document.createElement('span');
      const statusClass = entry.status >= 500 ? 'diag-status-error' : entry.status >= 400 ? 'diag-status-warn' : 'diag-status-ok';
      status.className = `diag-status ${statusClass}`;
      status.textContent = String(entry.status || '—');
      resultCell.append(status);
      row.append(resultCell);

      const request = document.createElement('td');
      const code = document.createElement('code');
      code.textContent = `${entry.method || ''} ${entry.path || ''}`.trim();
      request.append(code);
      if (entry.probe_id) {
        const probe = document.createElement('div');
        probe.className = 'small-text muted';
        probe.textContent = `诊断编号 ${entry.probe_id}`;
        request.append(probe);
      }
      row.append(request);

      appendCell(row, entry.kind === 'request' ? `${entry.duration_ms ?? 0} ms` : '—');
      const source = document.createElement('td');
      source.className = 'diag-source';
      source.textContent = [entry.host, entry.proto && `HTTPS 代理: ${entry.proto}`, entry.cloudflare_ray && `CF-Ray: ${entry.cloudflare_ray}`, entry.user_agent].filter(Boolean).join('\n');
      row.append(source);
      appendCell(row, entry.message || '—');
      body.append(row);
    }
  }

  async function load() {
    try {
      const response = await fetch('/admin/api/diagnostics/requests', { cache: 'no-store', credentials: 'same-origin' });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const result = await response.json();
      render(Array.isArray(result.entries) ? result.entries : []);
      updated.textContent = `已更新 ${new Date().toLocaleTimeString()}`;
    } catch (error) {
      updated.textContent = `读取失败：${error.message}`;
    }
  }

  refresh?.addEventListener('click', load);
  load();
  window.setInterval(load, 3000);
})();
