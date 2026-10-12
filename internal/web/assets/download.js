(() => {
  const stream = new EventSource('/download/events');
  let state;
  let snapshot;
  let changing = false;
  const ripples = new WeakMap();
  let syncMessage = document.getElementById('sync-state')?.textContent;
  const key = track => String(track.id);

  function ripple(element, event) {
    const bounds = element.getBoundingClientRect();
    const x = event.detail ? event.clientX - bounds.left : bounds.width / 2;
    const y = event.detail ? event.clientY - bounds.top : bounds.height / 2;
    element.style.setProperty('--ripple-x', `${x}px`);
    element.style.setProperty('--ripple-y', `${y}px`);
    element.classList.remove('rippling');
    void element.offsetWidth;
    element.classList.add('rippling');
    clearTimeout(ripples.get(element));
    ripples.set(element, setTimeout(() => element.classList.remove('rippling'), 600));
  }

  async function pingCurrentSource() {
    const content = document.getElementById('content');
    const source = content?.dataset.source;
    if (source !== 'spotify' && source !== 'vk') return;
    const button = content.querySelector('form[action="/ping"] button');
    if (!button || button.disabled) return;
    const field = content.querySelector('.connect .status span');
    if (field) field.textContent = 'Checking…';
    try {
      const response = await fetch('/ping', {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json'},
        body: new URLSearchParams({source})
      });
      if (!response.ok) throw new Error('connection check unavailable');
    } catch (_) {
      if (field?.isConnected) field.textContent = 'Could not start connection check. Try again.';
    }
  }

  function systemStatus() {
    const link = document.getElementById('system-status');
    const label = document.getElementById('system-status-text');
    if (!link || !label || !snapshot) return;
    let text;
    let short;
    let kind = 'idle';
    if (state?.stopped && state?.running) {
      text = 'Stopping…';
      short = 'Stopping';
      kind = 'active';
    } else if (state?.running) {
      text = `Syncing ${state.finished}/${state.total}`;
      short = 'Syncing';
      kind = 'active';
    } else if (snapshot.syncing) {
      text = 'Syncing library…';
      short = 'Syncing';
      kind = 'active';
    } else if (state?.stopped) {
      text = 'Syncing stopped';
      short = 'Stopped';
    } else if (snapshot.tracks.some(track => track.stage === 'failed')) {
      text = 'Syncing needs attention';
      short = 'Attention';
      kind = 'attention';
    } else if (!snapshot.library_count) {
      text = 'Sync a library to begin';
      short = 'Empty';
    } else if (!snapshot.tracks.length) {
      text = 'All tracks ready to play';
      short = 'Ready';
      kind = 'ready';
    } else {
      text = 'Syncing is not active';
      short = 'Idle';
    }
    label.textContent = short;
    const icon = kind === 'ready' ? 'checkmark-circle-02' : kind === 'active' || kind === 'attention' ? 'reload' : 'circle';
    link.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${icon}`);
    link.dataset.state = kind;
    link.dataset.syncing = snapshot.syncing ? 'true' : 'false';
    link.dataset.active = snapshot.syncing || (state?.running && !state.stopped) ? 'true' : 'false';
    link.title = `${text} · Open Downloads`;
    link.setAttribute('aria-label', link.title);
  }

  function control() {
    const button = document.getElementById('download-toggle');
    if (!button || !state) return;
    const enabled = !state.stopped;
    button.disabled = changing || (state.running && state.stopped);
    button.querySelector('.download-toggle-label').textContent = enabled ? 'Stop downloads' : 'Start downloads';
    button.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${enabled ? 'pause' : 'play'}`);
  }

  function summary() {
    const target = document.getElementById('download-state');
    if (!target || !state) return;
    target.textContent = state.running
      ? `${state.finished}/${state.total} · ${state.title ? `${state.artist} — ${state.title} · ` : ''}${state.stage || state.message}${state.total_bytes > 0 ? ` · ${Math.round(state.bytes / state.total_bytes * 100)}%` : ''}`
      : state.message;
  }

  function status(row, track) {
    const labels = {waiting: 'Waiting for sync', queued: 'Queued', checking: 'Checking', missing: 'Missing · downloading again', searching: 'Searching', downloading: 'Downloading', converting: 'Converting', metadata: 'Saving', retrying: 'Retrying', skipped: 'Saved', done: 'Saved', saved: 'Saved', failed: 'Failed'};
    const progress = track.total_bytes > 0 && !['stopped', 'failed'].includes(track.stage) ? ` · ${Math.round(track.bytes / track.total_bytes * 100)}%` : '';
    const field = row.querySelector('.download-stage');
    field.textContent = (labels[track.stage] || track.stage || 'Waiting') + progress;
    field.dataset.stage = track.stage;
    field.title = track.stage === 'failed' ? track.message || 'Download failed' : '';
  }

  function availability(track, playable) {
    for (const row of document.querySelectorAll('.library .tracks li[data-source]')) {
      if (row.dataset.id !== String(track.id)) continue;
      rowAvailability(row, playable);
    }
  }

  function rowAvailability(row, playable) {
    row.dataset.playable = String(playable);
    const button = row.querySelector('.play-track');
    if (button) button.disabled = !playable;
    const link = row.querySelector('.download-track');
    if (link) {
      link.classList.toggle('unavailable', !playable);
      if (playable) { link.removeAttribute('aria-disabled'); link.removeAttribute('tabindex'); }
      else { link.setAttribute('aria-disabled', 'true'); link.tabIndex = -1; }
    }
  }

  async function load() {
    const list = document.getElementById('download-tracks');
    try {
      const response = await fetch('/download/status');
      if (!response.ok) throw new Error('download status unavailable');
      snapshot = await response.json();
      state = snapshot.state;
      const available = new Set(snapshot.available_tracks.map(key));
      for (const row of document.querySelectorAll('.library:not(.download-library) li[data-id]')) {
        const wasPlayable = row.dataset.playable === 'true';
        const playable = available.has(row.dataset.id);
        rowAvailability(row, playable);
        if (wasPlayable && !playable) document.dispatchEvent(new CustomEvent('download:missing', {detail: {id: Number(row.dataset.id)}}));
        if (!wasPlayable && playable) document.dispatchEvent(new CustomEvent('download:done', {detail: {id: Number(row.dataset.id)}}));
      }
      document.dispatchEvent(new CustomEvent('download:catalog', {detail: snapshot.available_tracks}));
      systemStatus();
      control();
      if (!list || document.getElementById('download-tracks') !== list) return;
      summary();
      const mine = document.getElementById('my-download-tracks');
      if (mine) {
        mine.replaceChildren();
        for (const track of snapshot.my_tracks) mine.append(downloadRow(track));
        if (!snapshot.my_tracks.length) {
          const empty = document.createElement('li');
          empty.className = 'download-empty';
          empty.textContent = 'Your playlist is empty. Connect and sync a source.';
          mine.append(empty);
        }
      }
      list.replaceChildren();
      document.getElementById('download-count').textContent = `${snapshot.tracks.length} REMAINING`;
      for (const track of snapshot.tracks) {
        list.append(downloadRow(track));
      }
      if (!snapshot.tracks.length) {
        const empty = document.createElement('li');
        empty.className = 'download-empty';
        empty.textContent = snapshot.library_count ? 'All tracks are already downloaded.' : 'No tracks yet. Connect a source and sync its library.';
        list.append(empty);
      }
    } catch (_) {
      const target = document.getElementById('download-state');
      if (target) target.textContent = 'Could not load download status. Reopen Downloads to retry.';
      const label = document.getElementById('system-status-text');
      if (label) {
        label.textContent = 'Unknown';
        const link = document.getElementById('system-status');
        link.dataset.state = 'attention';
        link.dataset.syncing = 'false';
        link.dataset.active = 'false';
        link.querySelector('use').setAttribute('href', '/assets/icons.svg#hg-circle');
        link.title = 'Status unavailable · Open Downloads to retry';
        link.setAttribute('aria-label', link.title);
      }
    }
  }

  function downloadRow(track) {
    const row = document.createElement('li');
    row.dataset.id = track.id;
    const title = document.createElement('strong');
    title.textContent = track.title;
    const artist = document.createElement('span');
    artist.className = 'artist';
    artist.textContent = track.artist;
    const source = document.createElement('span');
    source.className = 'source-tag';
    source.textContent = track.source === 'spotify' ? 'Spotify' : 'VK Music';
    const stage = document.createElement('span');
    stage.className = 'download-stage';
    row.append(title, artist, source, stage);
    status(row, track);
    return row;
  }

  stream.onmessage = event => {
    try {
      const track = JSON.parse(event.data);
      availability(track, true);
      document.dispatchEvent(new CustomEvent('download:done', {detail: track}));
      load();
    } catch (_) { /* wait for next event */ }
  };
  stream.onerror = () => {
    const target = document.getElementById('download-state');
    if (target) target.textContent = 'Connection lost; reconnecting to download status…';
  };
  document.addEventListener('click', async event => {
    const button = event.target.closest('#download-toggle');
    if (!button || button.disabled || !state || changing) return;
    ripple(button, event);
    const action = state.stopped ? 'start' : 'stop';
    changing = true;
    control();
    try {
      const response = await fetch(`/download/${action}`, {method: 'POST'});
      if (!response.ok && response.status !== 409) throw new Error('request failed');
      await load();
    } catch (_) {
      const target = document.getElementById('download-state');
      if (target) target.textContent = `Could not ${action} downloads. Try again.`;
    } finally {
      changing = false;
      control();
    }
  });
  document.addEventListener('click', event => {
    const link = event.target.closest('#system-status');
    if (!link) return;
    const indicator = link.querySelector('.system-indicator');
    if (indicator) ripple(indicator, {detail: 0});
  });
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') {
      syncMessage = document.getElementById('sync-state')?.textContent;
      load();
      pingCurrentSource();
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
  pingCurrentSource();
  setInterval(load, 15000);
})();
