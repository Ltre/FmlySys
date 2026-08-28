self.addEventListener('push', (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch (_) {
    data = {body: event.data?.text() || '服药提醒'};
  }

  const title = data.title || '服药提醒';
  const options = {
    body: data.body || '请按计划服药',
    data: {
      url: data.url || '/medication',
    },
    tag: 'fmlysys-medication',
    renotify: true,
    silent: false,
  };

  event.waitUntil((async () => {
    const clientsList = await clients.matchAll({
      type: 'window',
      includeUncontrolled: true,
    });
    for (const client of clientsList) {
      client.postMessage({
        type: 'fmlysys-medication-reminder',
        voice: data.voice || data.body || '',
      });
    }
    await self.registration.showNotification(title, options);
  })());
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const target = event.notification.data?.url || '/medication';

  event.waitUntil((async () => {
    const clientsList = await clients.matchAll({
      type: 'window',
      includeUncontrolled: true,
    });
    for (const client of clientsList) {
      if ('focus' in client) {
        await client.focus();
        if ('navigate' in client) {
          await client.navigate(target);
        }
        return;
      }
    }
    await clients.openWindow(target);
  })());
});
