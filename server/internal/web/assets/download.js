(() => {
  const stream = new EventSource('/download/events');
  let state;
  let snapshot;
  let syncMessage = document.getElementById('sync-state')?.textContent;
  const key = track => `${track.source}\0${track.artist}\0${track.title}`;
  const rows = new Map();

  function systemStatus() {
    const link = document.getElementById('system-status');
    const label = document.getElementById('system-status-text');
    if (!link || !label || !snapshot) return;
    let text;
    let kind = 'idle';
    if (state?.running) {
      text = `Downloads ${state.finished}/${state.total}`;
      kind = 'active';
    } else if (snapshot.syncing) {
      text = 'Syncing library…';
      kind = 'active';
    } else if (snapshot.tracks.some(track => track.stage === 'failed')) {
      text = 'Downloads need attention';
      kind = 'attention';
    } else if (!snapshot.tracks.length) {
      text = 'Sync a library to begin';
    } else if (snapshot.tracks.every(track => ['done', 'skipped'].includes(track.stage))) {
      text = 'All tracks ready to play';
      kind = 'ready';
    } else {
      text = 'Sync to check tracks';
    }
    label.textContent = text;
    link.dataset.state = kind;
    link.title = `${text} · Open Downloads`;
  }

  function summary() {
    const target = document.getElementById('download-state');
    if (!target || !state) return;
    target.textContent = state.running
      ? `${state.finished}/${state.total} · ${state.title ? `${state.artist} — ${state.title} · ` : ''}${state.stage || state.message}${state.total_bytes > 0 ? ` · ${Math.round(state.bytes / state.total_bytes * 100)}%` : ''}`
      : state.message;
  }

  function status(row, track) {
    const labels = {waiting: 'Waiting for sync', queued: 'Queued', checking: 'Checking', searching: 'Searching', downloading: 'Downloading', converting: 'Converting', metadata: 'Saving', retrying: 'Retrying', skipped: 'Saved', done: 'Saved', saved: 'Saved', failed: 'Failed'};
    const progress = track.total_bytes > 0 ? ` · ${Math.round(track.bytes / track.total_bytes * 100)}%` : '';
    const field = row.querySelector('.download-stage');
    field.textContent = (labels[track.stage] || track.stage || 'Waiting') + progress;
    field.dataset.stage = track.stage;
    field.title = track.stage === 'failed' ? track.message || 'Download failed' : '';
  }

  async function load() {
    const list = document.getElementById('download-tracks');
    try {
      const response = await fetch('/download/status');
      if (!response.ok) throw new Error('download status unavailable');
      snapshot = await response.json();
      state = snapshot.state;
      systemStatus();
      if (!list || document.getElementById('download-tracks') !== list) return;
      summary();
      rows.clear();
      list.replaceChildren();
      document.getElementById('download-count').textContent = `${snapshot.tracks.length} TRACKS`;
      for (const track of snapshot.tracks) {
        const row = document.createElement('li');
        const title = document.createElement('strong');
        title.textContent = track.title;
        const artist = document.createElement('span');
        artist.className = 'artist';
        artist.textContent = track.artist;
        const source = document.createElement('span');
        source.className = 'source-tag';
        source.textContent = track.source === 'spotify' ? 'YouTube' : 'VK Music';
        const stage = document.createElement('span');
        stage.className = 'download-stage';
        row.append(title, artist, source, stage);
        status(row, track);
        list.append(row);
        rows.set(key(track), row);
      }
      if (!snapshot.tracks.length) {
        const empty = document.createElement('li');
        empty.className = 'download-empty';
        empty.textContent = 'No tracks yet. Connect a source and sync its library.';
        list.append(empty);
      }
    } catch (_) {
      const target = document.getElementById('download-state');
      if (target) target.textContent = 'Could not load download status. Reopen Downloads to retry.';
      const label = document.getElementById('system-status-text');
      if (label) label.textContent = 'Status unavailable';
    }
  }

  stream.onmessage = event => {
    try {
      state = JSON.parse(event.data);
      if (snapshot && state.title) {
        const track = snapshot.tracks.find(item => key(item) === key(state));
        if (track) Object.assign(track, {stage: state.stage, message: state.message, bytes: state.bytes, total_bytes: state.total_bytes});
      }
      systemStatus();
      summary();
      if (state.stage === 'queued' && !state.title) load();
      if (state.title && document.getElementById('download-tracks')) {
        const row = rows.get(key(state));
        if (row) status(row, state);
        else load();
      }
      if (state.stage === 'done') {
        document.dispatchEvent(new Event('download:done'));
        for (const row of document.querySelectorAll('.library .tracks li[data-source]')) {
          if (row.dataset.source === state.source && row.dataset.artist === state.artist && row.dataset.title === state.title) {
            row.dataset.playable = 'true';
            const button = row.querySelector('.play-track');
            if (button) button.disabled = false;
          }
        }
      }
    } catch (_) { /* wait for next event */ }
  };
  stream.onerror = () => {
    const target = document.getElementById('download-state');
    if (target) target.textContent = 'Connection lost; reconnecting to download status…';
  };
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') {
      syncMessage = document.getElementById('sync-state')?.textContent;
      load();
    }
    if (event.detail.target.id === 'sync-state') {
      const message = event.detail.target.textContent;
      if (message !== syncMessage && message.startsWith('Synced ')) {
        htmx.ajax('GET', location.pathname + location.search, {target: '#content', select: '#content', swap: 'outerHTML'});
      }
      syncMessage = message;
    }
  });
  load();
  setInterval(load, 15000);
})();
