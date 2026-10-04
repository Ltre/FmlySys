(() => {
  const body = document.getElementById('diagnostic-entries');
  const updated = document.getElementById('diagnostic-updated');
  const refresh = document.getElementById('refresh-diagnostics');
  const copyButton = document.getElementById('copy-diagnostics');
  const downloadButton = document.getElementById('download-diagnostics');
  if (!body || !updated) return;
  let latestEntries = [];

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
    latestEntries = entries;
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
      const kindLabels = {
        wechat_api: '微信 API',
        wechat_callback: '微信回调',
        http_error: 'HTTP 错误',
        panic: '服务异常',
        error: '登录错误',
      };
      const isError = ['http_error', 'panic', 'error'].includes(entry.kind);
      kind.className = `diag-kind ${isError ? 'diag-kind-error' : 'diag-kind-request'} ${entry.kind === 'wechat_api' || entry.kind === 'wechat_callback' ? 'diag-kind-wechat' : ''}`;
      kind.textContent = kindLabels[entry.kind] || entry.kind || '请求';
      resultCell.append(kind, document.createTextNode(' '));
      const status = document.createElement('span');
      const statusClass = entry.status >= 500 ? 'diag-status-error' : entry.status >= 400 ? 'diag-status-warn' : 'diag-status-ok';
      status.className = `diag-status ${statusClass}`;
      status.textContent = String(entry.status || '—');
      resultCell.append(status);
      row.append(resultCell);

      const request = document.createElement('td');
      const code = document.createElement('code');
      code.textContent = `${entry.method || ''} ${entry.url || entry.path || ''}`.trim();
      request.append(code);
      if (entry.probe_id) {
        const probe = document.createElement('div');
        probe.className = 'small-text muted';
        probe.textContent = `诊断编号 ${entry.probe_id}`;
        request.append(probe);
      }
      row.append(request);

      appendCell(row, Number.isFinite(entry.duration_ms) ? `${entry.duration_ms} ms` : '—');
      const source = document.createElement('td');
      source.className = 'diag-source';
      source.textContent = [entry.host, entry.proto && `HTTPS 代理: ${entry.proto}`, entry.cloudflare_ray && `CF-Ray: ${entry.cloudflare_ray}`, entry.user_agent].filter(Boolean).join('\n');
      row.append(source);

      const detailCell = document.createElement('td');
      if (entry.message) {
        const message = document.createElement('p');
        message.className = 'diag-error-message';
        message.textContent = entry.message + (entry.message_truncated ? ' [错误详情已截断]' : '');
        detailCell.append(message);
      }
      const details = document.createElement('details');
      const summary = document.createElement('summary');
      summary.textContent = '展开原始请求 / 响应';
      const pre = document.createElement('pre');
      pre.className = 'diag-raw-details';
      pre.textContent = JSON.stringify({
        kind: entry.kind,
        method: entry.method,
        path: entry.path,
        url: entry.url,
        url_truncated: entry.url_truncated,
        query: entry.query,
        query_truncated: entry.query_truncated,
        status: entry.status,
        duration_ms: entry.duration_ms,
        host: entry.host,
        remote_addr: entry.remote_addr,
        proto: entry.proto,
        cloudflare_ray: entry.cloudflare_ray,
        user_agent: entry.user_agent,
        request_headers: entry.request_headers,
        request_headers_truncated: entry.request_headers_truncated,
        request_body: entry.request_body,
        request_body_truncated: entry.request_body_truncated,
        response_headers: entry.response_headers,
        response_headers_truncated: entry.response_headers_truncated,
        response_body: entry.response_body,
        response_body_truncated: entry.response_body_truncated,
        error: entry.message,
        error_truncated: entry.message_truncated,
      }, null, 2);
      details.append(summary, pre);
      detailCell.append(details);
      row.append(detailCell);
      body.append(row);
    }
  }

  const diagnosticJSON = () => JSON.stringify(latestEntries, null, 2);

  copyButton?.addEventListener('click', async () => {
    const value = diagnosticJSON();
    try {
      await navigator.clipboard.writeText(value);
      updated.textContent = '原始诊断 JSON 已复制';
    } catch (_) {
      window.prompt('复制下面的原始诊断 JSON（可能包含 Cookie、验证码和令牌）：', value);
    }
  });

  downloadButton?.addEventListener('click', () => {
    const blob = new Blob([diagnosticJSON()], { type: 'application/json;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `fmlysys-diagnostics-${new Date().toISOString().replaceAll(':', '-')}.json`;
    link.click();
    URL.revokeObjectURL(url);
  });

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
