(() => {
  const imageName = /\.(?:jpe?g|png|gif|webp|bmp|heic|heif)$/i;
  const links = [...document.querySelectorAll('a.attachment[href^="/evidence/"], a.attachment[href^="/files/"], a.attachment[href^="/matter-files/"]')]
    .filter((link) => imageName.test(link.textContent.trim()));
  if (!links.length) return;

  const dialog = document.createElement('dialog');
  dialog.className = 'evidence-preview';
  dialog.setAttribute('aria-labelledby', 'evidence-preview-title');
  dialog.innerHTML = '<div class="evidence-preview-toolbar"><strong id="evidence-preview-title"></strong><button type="button" class="evidence-preview-close" aria-label="关闭图片预览">×</button></div><img class="evidence-preview-image" alt=""><a class="secondary-button evidence-preview-original" target="_blank" rel="noopener noreferrer">打开原图</a>';
  document.body.appendChild(dialog);
  const title = dialog.querySelector('#evidence-preview-title');
  const image = dialog.querySelector('.evidence-preview-image');
  const original = dialog.querySelector('.evidence-preview-original');
  dialog.querySelector('.evidence-preview-close').addEventListener('click', () => dialog.close());
  dialog.addEventListener('click', (event) => {
    if (event.target === dialog) dialog.close();
  });
  dialog.addEventListener('close', () => image.removeAttribute('src'));

  for (const link of links) {
    const name = link.textContent.trim();
    let previewable = true;
    const thumbnail = document.createElement('img');
    thumbnail.className = 'evidence-thumbnail';
    thumbnail.src = link.href;
    thumbnail.alt = '';
    thumbnail.loading = 'lazy';
    thumbnail.addEventListener('error', () => {
      previewable = false;
      thumbnail.remove();
      link.classList.remove('evidence-image-link');
      link.removeAttribute('aria-label');
    });
    link.prepend(thumbnail);
    link.classList.add('evidence-image-link');
    link.setAttribute('aria-label', `预览图片：${name}`);
    link.addEventListener('click', (event) => {
      if (!previewable || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      event.preventDefault();
      title.textContent = name;
      image.alt = name;
      image.src = link.href;
      original.href = link.href;
      if (!dialog.open) dialog.showModal();
    });
  }
})();
