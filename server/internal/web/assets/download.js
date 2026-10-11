(() => {
  const stream = new EventSource('/download/events');
  let state;
  function render() {
    const target = document.getElementById('download-state');
    if (!target || !state) return;
    const track = state.title ? ` · ${state.artist} — ${state.title}` : '';
    const progress = state.total_bytes > 0 ? ` · ${Math.round(state.bytes / state.total_bytes * 100)}%` : '';
    target.textContent = `${state.finished}/${state.total} · ${state.stage || state.message}${track}${progress}`;
    if (!state.running && state.stage === 'finished') target.textContent = state.message;
  }
  stream.onmessage = event => {
    try {
      state = JSON.parse(event.data);
      render();
      if (state.stage === 'done') {
        for (const row of document.querySelectorAll('.tracks li')) {
          if (row.dataset.source === state.source && row.dataset.artist === state.artist && row.dataset.title === state.title) {
            row.dataset.playable = 'true';
            const button = row.querySelector('.play-track');
            if (button) button.disabled = false;
            const download = row.querySelector('form button');
            if (download) download.textContent = 'Saved';
          }
        }
      }
    } catch (_) { /* wait for next event */ }
  };
  stream.onerror = () => {
    const target = document.getElementById('download-state');
    if (target) target.textContent = 'Connection lost; reconnecting to download status…';
  };
  function popup(message) {
    const dialog = document.getElementById('download-dialog');
    const text = document.getElementById('download-dialog-message');
    if (!dialog || !text || typeof dialog.showModal !== 'function') { window.alert(message); return; }
    text.textContent = message;
    if (!dialog.open) dialog.showModal();
  }
  document.addEventListener('click', event => {
    if (event.target.id === 'download-dialog-close') document.getElementById('download-dialog')?.close();
  });
  document.addEventListener('submit', async event => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement) || form.getAttribute('action') !== '/download') return;
    event.preventDefault();
    const button = event.submitter;
    if (button) button.disabled = true;
    try {
      const response = await fetch('/download', {
        method: 'POST',
        headers: {Accept: 'application/json'},
        body: new URLSearchParams(new FormData(form)),
      });
      if (response.status === 409) popup('Загрузка уже идёт. Дождитесь её завершения.');
      else if (response.status === 404) popup('Трек больше не найден в библиотеке. Обновите страницу.');
      else if (!response.ok) popup('Не удалось запустить загрузку. Попробуйте ещё раз.');
    } catch (_) {
      popup('Нет соединения с сервером. Попробуйте ещё раз.');
    } finally {
      if (button) button.disabled = false;
    }
  });
  document.addEventListener('htmx:afterSwap', render);
})();
