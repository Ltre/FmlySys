(() => {
  const imageName = /\.(?:jpe?g|png|gif|webp|bmp|heic|heif)$/i;
  const links = [...document.querySelectorAll('a.attachment[href^="/evidence/"], a.attachment[href^="/files/"], a.attachment[href^="/matter-files/"]')]
    .filter((link) => imageName.test(link.textContent.trim()));
  if (!links.length) return;

  // Table cells contain bare attachment links. Give each set its own wrapping
  // layout so multiple thumbnails stay together on narrow screens.
  for (const cell of new Set(links.map((link) => link.parentElement).filter((parent) => parent?.tagName === 'TD'))) {
    const attachments = [...cell.children].filter((child) => child.matches('a.attachment'));
    if (!attachments.length) continue;
    const gallery = document.createElement('div');
    gallery.className = 'attachment-gallery';
    cell.insertBefore(gallery, attachments[0]);
    for (const attachment of attachments) gallery.appendChild(attachment);
  }

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
    const name = link.textContent.trim().replace(/^📎\s*/, '');
    let previewable = true;
    const thumbnail = document.createElement('img');
    thumbnail.className = 'evidence-thumbnail';
    thumbnail.alt = '';
    thumbnail.loading = 'lazy';
    thumbnail.addEventListener('error', () => {
      previewable = false;
      link.textContent = name;
      link.classList.remove('evidence-image-link');
      link.removeAttribute('aria-label');
    });
    const caption = document.createElement('span');
    caption.className = 'evidence-image-name';
    caption.textContent = name;
    link.replaceChildren(thumbnail, caption);
    link.classList.add('evidence-image-link');
    link.title = name;
    link.setAttribute('aria-label', `预览图片：${name}`);
    thumbnail.src = link.href;
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
