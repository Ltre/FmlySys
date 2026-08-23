(() => {
  const workflowPaths = new Set([
    '/assets/self-events',
    '/assets/expenses',
    '/assets/transfers',
    '/assets/reimbursements',
    '/admin/assets/events',
    '/admin/assets/expenses',
    '/admin/assets/transfers',
    '/admin/assets/reimbursements'
  ]);

  function feedbackNode(form) {
    let node = form.querySelector(':scope > .form-feedback');
    if (!node) {
      node = document.createElement('div');
      node.className = 'form-feedback';
      node.setAttribute('aria-live', 'polite');
      form.prepend(node);
    }
    return node;
  }

  function setFeedback(form, message, isError = false) {
    const node = feedbackNode(form);
    node.textContent = message || '';
    node.hidden = !message;
    node.classList.toggle('is-error', isError);
    node.classList.toggle('is-success', !isError && Boolean(message));
  }

  function setBusy(form, submitter, busy) {
    form.dataset.workflowSubmitting = busy ? '1' : '0';
    form.querySelectorAll('button').forEach((button) => {
      if (busy) {
        button.dataset.workflowWasDisabled = button.disabled ? '1' : '0';
        button.disabled = true;
      } else {
        button.disabled = button.dataset.workflowWasDisabled === '1';
        delete button.dataset.workflowWasDisabled;
      }
    });
    if (!submitter) return;
    if (busy) {
      submitter.dataset.workflowOriginalText = submitter.textContent;
      submitter.textContent = '提交中…';
    } else if (submitter.dataset.workflowOriginalText) {
      submitter.textContent = submitter.dataset.workflowOriginalText;
      delete submitter.dataset.workflowOriginalText;
    }
  }

  function requestBody(form, submitter) {
    const data = new FormData(form);
    if (submitter && submitter.name) data.append(submitter.name, submitter.value);
    const hasSelectedFile = Array.from(form.querySelectorAll('input[type="file"]'))
      .some((input) => input.files && input.files.length > 0);
    if (hasSelectedFile) return data;

    const params = new URLSearchParams();
    for (const [key, value] of data.entries()) {
      if (typeof value === 'string') params.append(key, value);
    }
    return params;
  }

  function successMessage(path) {
    const messages = {
      '/assets/self-events': '资产变动已登记。',
      '/assets/expenses': '消费已保存，报销状态已重新计算。',
      '/assets/transfers': '成员转账已记录。',
      '/assets/reimbursements': '报销已登记，余额与待报销金额已更新。',
      '/admin/assets/events': '资产变动已登记。',
      '/admin/assets/expenses': '后台消费已保存，报销状态已重新计算。',
      '/admin/assets/transfers': '后台内部转账已记录。',
      '/admin/assets/reimbursements': '后台报销已登记。'
    };
    return messages[path] || '操作已完成。';
  }

  function navigateAfterSuccess(target) {
    const current = new URL(window.location.href);
    const sameDocument = target.origin === current.origin &&
      target.pathname === current.pathname &&
      target.search === current.search;
    if (sameDocument) {
      window.history.replaceState(null, '', target.href);
      window.location.reload();
      return;
    }
    window.location.assign(target.href);
  }

  document.addEventListener('submit', async (event) => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement)) return;
    const method = (form.getAttribute('method') || 'get').toLowerCase();
    if (method !== 'post') return;
    const path = new URL(form.action, window.location.href).pathname;
    if (!workflowPaths.has(path)) return;

    event.preventDefault();
    event.stopPropagation();
    if (form.dataset.workflowSubmitting === '1' || !form.reportValidity()) return;

    const submitter = event.submitter || null;
    setFeedback(form, '正在提交，请稍候…');
    setBusy(form, submitter, true);
    try {
      const response = await fetch(form.action, {
        method: 'POST',
        body: requestBody(form, submitter),
        credentials: 'same-origin',
        headers: {
          Accept: 'application/json',
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

      const message = successMessage(path);
      sessionStorage.setItem('fmlyFlash', JSON.stringify({ message, type: 'success' }));
      const target = new URL(payload.redirect || window.location.href, window.location.href);
      if (form.dataset.returnAnchor && target.origin === window.location.origin) {
        target.hash = form.dataset.returnAnchor;
      }
      navigateAfterSuccess(target);
    } catch (error) {
      setBusy(form, submitter, false);
      setFeedback(form, error instanceof Error ? error.message : '提交失败，请检查输入后重试。', true);
    }
  }, true);

  function prepareEvidenceLinks() {
    document.querySelectorAll('a.attachment[href^="/evidence/"]').forEach((link) => {
      link.target = '_blank';
      link.rel = 'noopener';
    });
  }

  function prepareEvidenceInputs() {
    document.querySelectorAll('input[type="file"][name="evidence"]').forEach((input) => {
      input.accept = 'image/*,video/*,audio/*,.pdf,.txt,.doc,.docx,.xls,.xlsx,.ppt,.pptx';
      const hint = input.closest('.upload')?.querySelector('span');
      if (hint) {
        hint.textContent = '支持图片、视频、音频、PDF、TXT、Word、Excel、PPT；单个文件最大 10MB';
      }
    });
  }

  function prepareAdminReimbursementJump() {
    const form = document.querySelector('form[action="/admin/assets/reimbursements"]');
    if (!form) return;
    const section = form.closest('.card');
    const select = form.querySelector('select[name="expense_id"]');
    if (!section || !select) return;
    section.id = 'reimbursement';
    select.id = 'reimbursement-expense';

    document.querySelectorAll('a[href^="/assets/expenses/"][href$="/edit"]').forEach((link) => {
      const path = new URL(link.href, window.location.href).pathname;
      const match = path.match(/^\/assets\/expenses\/(\d+)\/edit$/);
      const row = link.closest('tr');
      if (!match || !row || row.cells.length < 5) return;
      const cell = row.cells[4];
      if (!cell.textContent.includes('待 ¥') || cell.querySelector('[data-reimburse-expense]')) return;

      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'secondary-button reimbursement-jump';
      button.dataset.reimburseExpense = match[1];
      button.textContent = '报销';
      cell.append(document.createElement('br'), button);
    });
  }

  prepareEvidenceLinks();
  prepareEvidenceInputs();
  prepareAdminReimbursementJump();
})();
