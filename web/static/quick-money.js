(() => {
  const boxes = Array.from(document.querySelectorAll('[data-quick-category]'));
  boxes.forEach((box) => {
    box.addEventListener('change', () => {
      if (!box.checked) return;
      boxes.forEach((other) => {
        if (other !== box) other.checked = false;
      });
    });
  });

  const form = document.querySelector('form[action="/quick-money-note"]');
  if (form) {
    form.addEventListener('submit', (event) => {
      if (boxes.filter((box) => box.checked).length !== 1) {
        event.preventDefault();
        event.stopImmediatePropagation();
        alert('请选择且只能选择一个记录分类。');
        boxes[0]?.focus();
      }
    }, true);
  }

  document.querySelectorAll('[data-quick-created-at]').forEach((node) => {
    const date = new Date(node.dataset.quickCreatedAt || '');
    if (Number.isNaN(date.getTime())) {
      node.textContent = node.dataset.quickCreatedAt || '';
      return;
    }
    const pad = (value) => String(value).padStart(2, '0');
    node.textContent = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
  });
})();
