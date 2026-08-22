(() => {
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

  enhanceResponsiveTables();
  enhanceMemberDelete();
  enhanceFileInputs();
  enableAdminTap();
})();
