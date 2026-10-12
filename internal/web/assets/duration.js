(() => {
  const cache = new Map();
  const queue = [];
  let active = 0;
  let observer;
  const key = row => String(row.dataset.id);
  const url = row => `/audio?${new URLSearchParams({id: row.dataset.id})}`;

  function next() {
    while (active < 3 && queue.length) {
      const row = queue.shift();
      if (!row.isConnected || row.dataset.playable !== 'true') continue;
      active++;
      const audio = new Audio();
      const finish = seconds => {
        audio.onloadedmetadata = null;
        audio.onerror = null;
        if (Number.isFinite(seconds) && seconds > 0 && row.isConnected && row.dataset.playable === 'true') {
          const minutes = Math.floor(seconds / 60);
          const duration = `${minutes}:${String(Math.floor(seconds % 60)).padStart(2, '0')}`;
          cache.set(key(row), duration);
          row.querySelector('.track-duration').textContent = duration;
        }
        audio.removeAttribute('src');
        audio.load();
        active--;
        next();
      };
      audio.onloadedmetadata = () => finish(audio.duration);
      audio.onerror = () => finish(NaN);
      audio.preload = 'metadata';
      audio.src = url(row);
    }
  }

  function watch(row) {
    if (row.dataset.playable !== 'true') return;
    const saved = cache.get(key(row));
    if (saved) { row.querySelector('.track-duration').textContent = saved; return; }
    observer.observe(row);
  }

  function init() {
    observer?.disconnect();
    queue.length = 0;
    observer = new IntersectionObserver(entries => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        observer.unobserve(entry.target);
        queue.push(entry.target);
      }
      next();
    });
    document.querySelectorAll('.library:not(.download-library) .tracks li[data-source]').forEach(watch);
  }

  document.addEventListener('htmx:afterSwap', event => { if (event.detail.target.id === 'content') init(); });
  for (const [event, playable] of [['download:done', true], ['download:missing', false]]) {
    document.addEventListener(event, e => {
      const track = e.detail;
      if (!track) return;
      for (const row of document.querySelectorAll('.library:not(.download-library) .tracks li[data-source]')) {
        if (key(row) !== String(track.id)) continue;
        cache.delete(key(row));
        row.querySelector('.track-duration').textContent = '—';
        if (playable) queueMicrotask(() => watch(row));
      }
    });
  }
  init();
})();
