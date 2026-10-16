(() => {
  const stream = new EventSource('/download/events');
  let state;
  let snapshot;
  let changing = false;
  const ripples = new WeakMap();
  let syncMessage = document.getElementById('sync-state')?.textContent;
  const key = track => String(track.id);
  const pending = track => !track.playable && !['downloaded', 'done', 'saved', 'skipped'].includes(track.stage);

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
    const button = content.querySelector('[data-source-ping] button');
    if (!button || button.disabled) return;
    const field = document.getElementById('connection-state');
    if (field) {
      field.textContent = 'Checking…';
      field.dataset.error = 'false';
      field.setAttribute('role', 'status');
    }
    try {
      const response = await fetch('/ping', {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json'},
        body: new URLSearchParams({source})
      });
      if (!response.ok) throw new Error('connection check unavailable');
    } catch (_) {
      if (field?.isConnected) {
        field.textContent = 'Could not start connection check. Try again.';
        field.dataset.error = 'true';
        field.setAttribute('role', 'alert');
      }
    }
  }

  function sidebarSummary() {
    if (!snapshot?.library) return;
    const library = snapshot.library;
    for (const field of document.querySelectorAll('[data-catalog-count]')) field.textContent = library.total;
    const personal = document.getElementById('sidebar-personal-count');
    if (personal) personal.textContent = library.personal;
    const youtubeCount = document.querySelector('[data-source-count="youtube"]');
    if (youtubeCount) youtubeCount.textContent = library.youtube;
    for (const source of ['spotify', 'vk']) {
      const connected = library[`${source}_connected`];
      const link = document.querySelector(`.sidebar-source[data-source="${source}"]`);
      if (link) link.hidden = !connected;
      const count = document.querySelector(`[data-source-count="${source}"]`);
      if (count) count.textContent = library[source];
      const connect = document.querySelector(`[data-connect-source="${source}"]`);
      if (connect) connect.hidden = connected;
    }
    const empty = document.getElementById('sidebar-no-sources');
    if (empty) empty.hidden = library.spotify_connected || library.vk_connected;
    const connect = document.getElementById('sidebar-connect');
    if (connect) connect.hidden = library.spotify_connected && library.vk_connected;
    const sync = document.getElementById('sidebar-sync-status');
    if (sync) {
      const status = snapshot.library_sync;
      const active = ['Syncing', 'Downloading', 'Stopping'].includes(status);
      sync.querySelector('span').textContent = status;
      sync.dataset.state = status === 'Attention' ? 'attention' : active ? 'active' : 'ready';
      sync.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${active ? 'reload' : status === 'Ready' ? 'checkmark-circle-02' : 'circle'}`);
      sync.title = `Library synchronization and downloads across all users: ${status}. Open Downloads`;
      sync.setAttribute('aria-label', sync.title);
    }
  }

  function sidebarNavigation() {
    const source = document.getElementById('content')?.dataset.source;
    for (const link of document.querySelectorAll('.sidebar nav a')) {
      const selected = (new URL(link.href).searchParams.get('source') || 'music') === source;
      link.classList.toggle('active', selected);
      if (selected) link.setAttribute('aria-current', 'page');
      else link.removeAttribute('aria-current');
    }
  }

  function control() {
    const button = document.getElementById('download-toggle');
    if (!button || !state) return;
    const enabled = !state.stopped;
    button.disabled = changing || (state.running && state.stopped);
    button.querySelector('.download-toggle-label').textContent = enabled ? 'Stop downloads' : 'Start downloads';
    button.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${enabled ? 'pause' : 'play'}`);
    button.classList.toggle('primary', !enabled);
  }

  function summary() {
    const target = document.getElementById('download-state');
    if (!target || !state) return;
    const remaining = snapshot.tracks.filter(pending);
    const failed = remaining.filter(track => track.stage === 'failed').length;
    const active = remaining.filter(track => ['checking', 'searching', 'downloading', 'converting', 'metadata', 'retrying'].includes(track.stage)).length;
    const waiting = remaining.length - failed - active;
    const parts = [];
    if (failed) parts.push(`${failed} failed`);
    if (active) parts.push(`${active} in progress`);
    if (waiting) parts.push(`${waiting} ${state.stopped ? 'stopped' : 'queued'}`);
    target.textContent = parts.join(' · ') || 'No pending downloads.';
    target.dataset.error = 'false';
    target.setAttribute('role', 'status');
    const field = document.getElementById('download-summary-status');
    if (field) {
      const label = state.stopped ? (state.running ? 'Stopping' : 'Stopped') : failed ? 'Needs attention' : state.running ? 'Downloading' : 'Idle';
      field.querySelector('span').textContent = label;
      field.dataset.state = failed ? 'failed' : '';
      field.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${state.running ? 'reload' : 'circle'}`);
    }
  }

  function status(row, track) {
    const labels = {waiting: 'Not downloaded', queued: 'Queued', checking: 'Checking', missing: 'Missing · downloading again', searching: 'Searching', downloading: 'Downloading', converting: 'Converting', metadata: 'Saving', retrying: 'Retrying', skipped: 'Ready', done: 'Ready', saved: 'Ready', downloaded: 'Ready', stopped: 'Stopped', failed: 'Failed'};
    const stage = state?.stopped && ['waiting', 'queued'].includes(track.stage) ? 'stopped' : track.stage;
    const percent = track.total_bytes > 0 && ['downloading', 'converting', 'metadata'].includes(stage)
      ? Math.max(0, Math.min(100, Math.round((track.bytes || 0) / track.total_bytes * 100))) : null;
    const field = row.querySelector('.download-stage');
    if (!field) return;
    field.querySelector('.status-label').textContent = (labels[stage] || stage || 'Waiting') + (percent === null ? '' : ` · ${percent}%`);
    field.dataset.stage = stage;
    field.title = stage === 'failed' ? track.message || 'Download failed' : '';
    const ready = ['downloaded', 'done', 'saved', 'skipped'].includes(stage);
    const active = ['checking', 'searching', 'downloading', 'converting', 'metadata', 'retrying'].includes(stage);
    field.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${ready ? 'checkmark-circle-02' : active ? 'reload' : 'circle'}`);
    const progress = row.querySelector('progress');
    if (progress) {
      progress.hidden = percent === null;
      progress.value = percent || 0;
    }
  }

  function availability(track, playable) {
    for (const row of document.querySelectorAll('.library .tracks li[data-source]')) {
      if (row.dataset.id !== String(track.id)) continue;
      rowAvailability(row, playable);
      if (playable) status(row, {...track, stage: 'downloaded'});
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
      const personal = new Map(snapshot.my_tracks.map(track => [key(track), track]));
      const queued = new Map(snapshot.tracks.map(track => [key(track), track]));
      const catalog = document.getElementById('content')?.dataset.source === 'all';
      for (const row of document.querySelectorAll('.library:not(.download-library) li[data-id]')) {
        const wasPlayable = row.dataset.playable === 'true';
        const track = personal.get(row.dataset.id) || queued.get(row.dataset.id);
        const playable = track ? Boolean(track.playable) : catalog ? wasPlayable : available.has(row.dataset.id);
        rowAvailability(row, playable);
        status(row, playable ? {stage: 'downloaded'} : track || {stage: 'waiting'});
        if (wasPlayable && !playable) document.dispatchEvent(new CustomEvent('download:missing', {detail: {id: Number(row.dataset.id)}}));
        if (!wasPlayable && playable) document.dispatchEvent(new CustomEvent('download:done', {detail: {id: Number(row.dataset.id)}}));
      }
      document.dispatchEvent(new CustomEvent('download:catalog', {detail: snapshot.available_tracks}));
      sidebarSummary();
      control();
      if (!list || document.getElementById('download-tracks') !== list) return;
      summary();
      const mine = document.getElementById('my-download-tracks');
      const myPending = snapshot.my_tracks.filter(pending);
      if (mine) {
        mine.replaceChildren();
        for (const track of myPending) mine.append(downloadRow(track));
        if (!myPending.length) mine.append(emptyRow('No pending downloads in your collection.'));
      }
      const count = document.getElementById('my-download-count');
      if (count) count.textContent = `${myPending.length} pending`;
      list.replaceChildren();
      const sharedPending = snapshot.tracks.filter(pending);
      document.getElementById('download-count').textContent = `${sharedPending.length} pending`;
      for (const track of sharedPending) {
        list.append(downloadRow(track));
      }
      if (!sharedPending.length) list.append(emptyRow('No pending downloads on this server.'));
    } catch (_) {
      const target = document.getElementById('download-state');
      if (target) {
        target.textContent = 'Could not load download status. Reopen Downloads to retry.';
        target.dataset.error = 'true';
        target.setAttribute('role', 'alert');
      }
      const sync = document.getElementById('sidebar-sync-status');
      if (sync) {
        sync.querySelector('span').textContent = 'Unknown';
        sync.dataset.state = 'attention';
        sync.querySelector('use').setAttribute('href', '/assets/icons.svg#hg-circle');
        sync.title = 'Sync status unavailable. Open Downloads to retry';
        sync.setAttribute('aria-label', sync.title);
      }
    }
  }

  function downloadRow(track) {
    const row = document.getElementById('download-row-template').content.firstElementChild.cloneNode(true);
    row.dataset.id = track.id;
    row.querySelector('.track-title').textContent = track.title;
    row.querySelector('.artist').textContent = track.artist || 'Unknown artist';
    row.querySelector('.source-name').textContent = {spotify: 'Spotify', youtube: 'YouTube', vk: 'VK Music'}[track.source] || track.source;
    row.querySelector('.source-label use').setAttribute('href', `/assets/icons.svg#hg-${['spotify', 'youtube', 'vk'].includes(track.source) ? track.source : 'music-note-01'}`);
    if (track.cover) {
      const cover = row.querySelector('.track-cover img');
      cover.src = track.cover;
      cover.hidden = false;
    }
    status(row, track);
    return row;
  }

  function emptyRow(message) {
    const row = document.createElement('li');
    row.className = 'empty';
    const title = document.createElement('strong');
    title.textContent = message;
    row.append(title);
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
    if (target) {
      target.textContent = 'Connection lost; reconnecting to download status…';
      target.dataset.error = 'true';
      target.setAttribute('role', 'alert');
    }
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
      if (target) {
        target.textContent = `Could not ${action} downloads. Try again.`;
        target.dataset.error = 'true';
        target.setAttribute('role', 'alert');
      }
    } finally {
      changing = false;
      control();
    }
  });
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') {
      sidebarNavigation();
      syncMessage = document.getElementById('sync-state')?.textContent;
      load();
      pingCurrentSource();
    }
    if (event.detail.target.id === 'sync-state') {
      const message = event.detail.target.textContent.trim();
      if (message !== syncMessage && (syncMessage === 'Syncing…' || message === 'Syncing…' || message.startsWith('Synced ') || message.startsWith('Library synced, but'))) {
        htmx.ajax('GET', location.pathname + location.search, {target: '#content', select: '#content', swap: 'outerHTML'});
      }
      syncMessage = message;
    }
  });
  document.addEventListener('htmx:historyRestore', event => {
    const content = document.getElementById('content');
    if (!content) return;
    const source = new URL(event.detail.path, location.href).searchParams.get('source');
    content.dataset.source = ['all', 'spotify', 'vk', 'youtube', 'downloads', 'settings'].includes(source) ? source : 'music';
    sidebarNavigation();
    syncMessage = document.getElementById('sync-state')?.textContent;
    load();
    pingCurrentSource();
  });
  sidebarNavigation();
  load();
  pingCurrentSource();
  setInterval(load, 15000);
})();
