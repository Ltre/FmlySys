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
})();
