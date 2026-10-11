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
    try { state = JSON.parse(event.data); render(); } catch (_) { /* wait for next event */ }
  };
  stream.onerror = () => {
    const target = document.getElementById('download-state');
    if (target) target.textContent = 'Connection lost; reconnecting to download status…';
  };
  document.addEventListener('htmx:afterSwap', render);
})();
