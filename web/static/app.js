(() => {
  function sortAssetMovements() {
    const tbody = document.querySelector('#asset-movement-table tbody');
    if (!tbody) return;
    const rows = Array.from(tbody.querySelectorAll('tr[data-occurred-at]'));
    rows.sort((a, b) => {
      const aValue = a.dataset.occurredAt || '';
      const bValue = b.dataset.occurredAt || '';
      const aTime = Date.parse(aValue);
      const bTime = Date.parse(bValue);
      if (Number.isFinite(aTime) && Number.isFinite(bTime) && aTime !== bTime) {
        return bTime - aTime;
      }
      return bValue.localeCompare(aValue);
    });
    rows.forEach((row) => tbody.appendChild(row));
    if (!rows.length) {
      const row = document.createElement('tr');
      const cell = document.createElement('td');
      cell.colSpan = 5;
      cell.textContent = '暂无记录';
      row.appendChild(cell);
      tbody.appendChild(row);
    }
  }

  function enhanceResponsiveTables() {
    document.querySelectorAll('table').forEach((table) => {
      if (table.dataset.mobileEnhanced === '1' || !table.rows.length) return;
      const headerRow = table.rows[0];
      const labels = Array.from(headerRow.cells, (cell) => cell.textContent.trim());
      if (!labels.some(Boolean)) return;

      table.dataset.mobileEnhanced = '1';
      table.classList.add('responsive-table');
      headerRow.classList.add('responsive-table-head');

      Array.from(table.rows).slice(1).forEach((row) => {
        row.classList.add('responsive-table-row');
        Array.from(row.cells).forEach((cell, index) => {
          if (cell.colSpan > 1) {
            cell.classList.add('responsive-empty');
            return;
          }
          cell.dataset.label = labels[index] || '';
        });
      });
    });
  }

  function enhanceMemberDelete() {
    document.querySelectorAll('form.member-permission-card[action$="/permissions"]').forEach((form) => {
      if (form.dataset.deleteEnhanced === '1') return;
      form.dataset.deleteEnhanced = '1';

      const saveButton = Array.from(form.children).find((node) => node.tagName === 'BUTTON');
      if (!saveButton) return;

      const actions = document.createElement('div');
      actions.className = 'member-actions';
      saveButton.parentNode.insertBefore(actions, saveButton);
      actions.appendChild(saveButton);

      const deleteButton = document.createElement('button');
      deleteButton.type = 'submit';
      deleteButton.name = 'member_action';
      deleteButton.value = 'delete';
      deleteButton.className = 'danger-button';
      deleteButton.textContent = '删除成员';
      deleteButton.addEventListener('click', (event) => {
        if (!window.confirm('确认删除该成员？若存在历史关联数据将执行软删除，否则会直接删除成员记录。')) {
          event.preventDefault();
        }
      });
      actions.appendChild(deleteButton);
    });
  }

  function enhanceFileInputs() {
    document.querySelectorAll('input[type="file"]').forEach((input) => {
      if (input.dataset.selectionEnhanced === '1') return;
      input.dataset.selectionEnhanced = '1';
      const info = document.createElement('span');
      info.className = 'file-selection';
      info.setAttribute('aria-live', 'polite');
      input.insertAdjacentElement('afterend', info);
      input.addEventListener('change', () => {
        const files = Array.from(input.files || []);
        if (!files.length) {
          info.textContent = '';
          return;
        }
        const names = files.slice(0, 3).map((file) => file.name).join('、');
        info.textContent = files.length <= 3 ? `已选择 ${files.length} 个文件：${names}` : `已选择 ${files.length} 个文件：${names} 等`;
      });
    });
  }

  function enhanceReimbursementJumps() {
    const section = document.querySelector('#reimbursement');
    const select = document.querySelector('#reimbursement-expense');
    if (!section || !select) return;

    document.querySelectorAll('[data-reimburse-expense]').forEach((button) => {
      button.addEventListener('click', () => {
        select.value = button.dataset.reimburseExpense || '';
        select.dispatchEvent(new Event('change', { bubbles: true }));
        section.classList.add('interaction-highlight');
        section.scrollIntoView({ behavior: 'smooth', block: 'start' });
        window.setTimeout(() => {
          select.focus({ preventScroll: true });
          section.classList.remove('interaction-highlight');
        }, 420);
      });
    });
  }

  function getFormFeedback(form) {
    let node = form.querySelector(':scope > .form-feedback');
    if (!node) {
      node = document.createElement('div');
      node.className = 'form-feedback';
      node.setAttribute('aria-live', 'polite');
      form.prepend(node);
    }
    return node;
  }

  function setFormFeedback(form, message, type) {
    const node = getFormFeedback(form);
    node.textContent = message || '';
    node.classList.toggle('is-error', type === 'error');
    node.classList.toggle('is-success', type === 'success');
    node.hidden = !message;
    if (message && type === 'error') {
      node.setAttribute('role', 'alert');
      node.tabIndex = -1;
      node.focus({ preventScroll: true });
      node.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    } else {
      node.setAttribute('role', 'status');
      node.removeAttribute('tabindex');
    }
  }

  function successMessage(form, submitter) {
    if (submitter && submitter.name === 'member_action' && submitter.value === 'delete') {
      return '成员已删除；如存在历史关联数据，系统已保留为历史成员。';
    }
    if (form.dataset.success) return form.dataset.success;

    const path = new URL(form.action, window.location.href).pathname;
    const exact = {
      '/assets/self-events': '资产变动已登记。',
      '/assets/expenses': '消费已保存，报销状态已重新计算。',
      '/assets/transfers': '成员转账已记录。',
      '/assets/reimbursements': '报销已登记，余额与待报销金额已更新。',
      '/admin/members': '成员已添加。',
      '/admin/assets/events': '资产变动已登记。',
      '/admin/assets/expenses': '后台消费已保存，报销状态已重新计算。',
      '/admin/assets/transfers': '后台内部转账已记录。',
      '/admin/assets/reimbursements': '后台报销已登记。',
      '/matters': '家族事务已创建。',
      '/share': '共享资料已创建。'
    };
    if (exact[path]) return exact[path];
    if (/^\/admin\/members\/\d+\/permissions$/.test(path)) return '成员权限已保存。';
    if (/^\/admin\/join\/\d+\/approve$/.test(path)) return '加入申请已审核通过。';
    if (/^\/admin\/join\/\d+\/reject$/.test(path)) return '加入申请已拒绝。';
    if (/^\/matters\/\d+\/status$/.test(path)) return '事务状态已更新。';
    if (/^\/share\/\d+\/attachments$/.test(path)) return '附件已上传。';
    if (/^\/assets\/expenses\/\d+$/.test(path)) return '消费记录已修改，报销状态已重新计算。';
    return '操作已完成。';
  }

  function setSubmitBusy(form, submitter, busy) {
    form.dataset.submitting = busy ? '1' : '0';
    form.querySelectorAll('button').forEach((button) => {
      if (busy) {
        button.dataset.wasDisabled = button.disabled ? '1' : '0';
        button.disabled = true;
      } else {
        button.disabled = button.dataset.wasDisabled === '1';
        delete button.dataset.wasDisabled;
      }
    });
    if (!submitter) return;
    if (busy) {
      submitter.dataset.originalText = submitter.textContent;
      submitter.textContent = '提交中…';
      submitter.setAttribute('aria-busy', 'true');
    } else {
      if (submitter.dataset.originalText) submitter.textContent = submitter.dataset.originalText;
      delete submitter.dataset.originalText;
      submitter.removeAttribute('aria-busy');
    }
  }

  function shouldEnhanceForm(form) {
    const method = (form.getAttribute('method') || 'get').toLowerCase();
    if (method !== 'post') return false;
    if (form.classList.contains('nav-form') || form.dataset.noAsync === '1') return false;
    const path = new URL(form.action, window.location.href).pathname;
    return path !== '/logout' && path !== '/admin/logout';
  }

  function enhanceAsyncForms() {
    document.querySelectorAll('form').forEach((form) => {
      if (!shouldEnhanceForm(form) || form.dataset.asyncEnhanced === '1') return;
      form.dataset.asyncEnhanced = '1';

      form.addEventListener('submit', async (event) => {
        if (form.dataset.submitting === '1') {
          event.preventDefault();
          return;
        }
        event.preventDefault();
        if (!form.reportValidity()) return;

        const submitter = event.submitter || null;
        const data = new FormData(form);
        if (submitter && submitter.name) {
          data.append(submitter.name, submitter.value);
        }

        setFormFeedback(form, '正在提交，请稍候…', '');
        setSubmitBusy(form, submitter, true);

        try {
          const response = await fetch(form.action, {
            method: 'POST',
            body: data,
            credentials: 'same-origin',
            headers: {
              'Accept': 'application/json',
              'X-Fmly-Async': '1'
            }
          });
          const contentType = response.headers.get('content-type') || '';
          if (!contentType.includes('application/json')) {
            const text = (await response.text()).trim();
            throw new Error(text || `请求失败（HTTP ${response.status}）`);
          }
          const payload = await response.json();
          if (!response.ok || !payload.ok) {
            throw new Error(payload.message || `请求失败（HTTP ${response.status}）`);
          }

          const message = successMessage(form, submitter);
          setFormFeedback(form, message, 'success');
          sessionStorage.setItem('fmlyFlash', JSON.stringify({ message, type: 'success' }));

          const target = new URL(payload.redirect || window.location.href, window.location.href);
          if (form.dataset.returnAnchor && target.origin === window.location.origin) {
            target.hash = form.dataset.returnAnchor;
          }
          window.location.assign(target.href);
        } catch (error) {
          setSubmitBusy(form, submitter, false);
          setFormFeedback(form, error instanceof Error ? error.message : '提交失败，请检查输入后重试。', 'error');
        }
      });
    });
  }

  function showToast(message, type) {
    if (!message) return;
    document.querySelectorAll('.app-toast').forEach((node) => node.remove());

    const toast = document.createElement('div');
    toast.className = `app-toast ${type === 'error' ? 'is-error' : 'is-success'}`;
    toast.setAttribute('role', type === 'error' ? 'alert' : 'status');

    const text = document.createElement('span');
    text.textContent = message;
    toast.appendChild(text);

    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'toast-close';
    close.setAttribute('aria-label', '关闭提示');
    close.textContent = '×';
    close.addEventListener('click', () => toast.remove());
    toast.appendChild(close);

    document.body.appendChild(toast);
    window.setTimeout(() => toast.remove(), 5200);
  }

  function consumeFlash() {
    const raw = sessionStorage.getItem('fmlyFlash');
    if (!raw) return;
    sessionStorage.removeItem('fmlyFlash');
    try {
      const flash = JSON.parse(raw);
      showToast(flash.message, flash.type);
    } catch {
      showToast(raw, 'success');
    }
  }

  function enableAdminTap() {
    const header = document.querySelector('[data-admin-tap]');
    if (!header) return;

    let tapCount = 0;
    let lastTapAt = 0;
    const maxGapMs = 700;

    header.addEventListener('click', (event) => {
      if (typeof event.button === 'number' && event.button !== 0) return;
      if (event.target.closest('a,button,input,select,textarea,label,form,summary')) return;

      const now = performance.now();
      tapCount = now - lastTapAt <= maxGapMs ? tapCount + 1 : 1;
      lastTapAt = now;
      if (tapCount < 7) return;

      tapCount = 0;
      lastTapAt = 0;
      const opened = window.open('/admin', '_blank', 'noopener,noreferrer');
      if (opened) opened.opener = null;
    });
  }

  sortAssetMovements();
  enhanceResponsiveTables();
  enhanceMemberDelete();
  enhanceFileInputs();
  enhanceReimbursementJumps();
  enhanceAsyncForms();
  consumeFlash();
  enableAdminTap();
})();
