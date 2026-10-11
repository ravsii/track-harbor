(() => {
  const audio = document.getElementById('player-audio');
  if (!audio) return;

  const $ = id => document.getElementById(id);
  const shuffleButton = $('player-shuffle');
  const shuffleKey = 'trackharbor-shuffle';
  const stamp = track => `${track.source}\0${track.artist}\0${track.title}`;
  const time = seconds => Number.isFinite(seconds) ? `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}` : '0:00';
  let tracks = [], current, history = [], previousVolume = 0.8;
  audio.volume = 0.8;

  function setShuffle(enabled) {
    shuffleButton.setAttribute('aria-pressed', String(enabled));
    shuffleButton.title = enabled ? 'Shuffle on' : 'Shuffle off';
  }

  try {
    setShuffle(localStorage.getItem(shuffleKey) === 'true');
  } catch (_) {
    // Private browsing can disable storage; shuffle still works for this page.
  }

  function icon(id, name) {
    $(id).querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${name}`);
  }

  function updateVolume() {
    const muted = audio.muted || audio.volume === 0;
    icon('player-mute', muted ? 'volume-mute' : 'volume-high');
    $('player-mute').setAttribute('aria-label', muted ? 'Unmute' : 'Mute');
    $('player-mute').title = muted ? 'Unmute' : 'Mute';
    $('player-volume').value = muted ? 0 : Math.round(audio.volume * 100);
  }

  function refresh() {
    icon('player-toggle', audio.paused ? 'play' : 'pause');
    $('player-toggle').setAttribute('aria-label', audio.paused ? 'Play' : 'Pause');
    $('player-time').textContent = `${time(audio.currentTime)} / ${time(audio.duration)}`;
    $('player-seek').value = Number.isFinite(audio.duration) && audio.duration > 0
      ? Math.round(audio.currentTime / audio.duration * 1000) : 0;
  }

  async function loadTracks() {
    try {
      const response = await fetch('/audio/tracks');
      if (!response.ok) throw new Error('unavailable');
      tracks = await response.json();
    } catch (_) {
      $('player-error').textContent = 'Could not load saved tracks. Refresh the page to retry.';
    }
  }

  async function play(track, remember = true) {
    if (!track) {
      $('player-error').textContent = 'Download a track before playing it.';
      return;
    }
    if (remember && current && current !== stamp(track)) history.push(current);
    current = stamp(track);
    const params = new URLSearchParams({source: track.source, artist: track.artist, title: track.title});
    audio.src = `/audio?${params}`;
    $('player-track').textContent = `${track.artist} — ${track.title}`;
    $('player-error').textContent = '';
    try { await audio.play(); } catch (_) { $('player-error').textContent = 'Could not play this MP3. Check that it is still downloaded.'; }
    refresh();
  }

  async function next() {
    if (!tracks.length) await loadTracks();
    if (!tracks.length) { await play(null); return; }
    const index = tracks.findIndex(track => stamp(track) === current);
    if (shuffleButton.getAttribute('aria-pressed') === 'true' && tracks.length > 1) {
      const candidates = tracks.filter(track => stamp(track) !== current);
      await play(candidates[Math.floor(Math.random() * candidates.length)]);
    } else {
      await play(tracks[(index + 1) % tracks.length]);
    }
  }

  async function previous() {
    if (audio.currentTime > 3) { audio.currentTime = 0; return; }
    if (!tracks.length) await loadTracks();
    if (!tracks.length) { await play(null); return; }
    const prior = history.pop();
    const index = tracks.findIndex(track => stamp(track) === current);
    await play((prior && tracks.find(track => stamp(track) === prior)) ||
      tracks[(index - 1 + tracks.length) % tracks.length], false);
  }

  audio.addEventListener('timeupdate', refresh);
  audio.addEventListener('durationchange', refresh);
  audio.addEventListener('play', refresh);
  audio.addEventListener('pause', refresh);
  audio.addEventListener('volumechange', updateVolume);
  audio.addEventListener('error', () => { $('player-error').textContent = 'MP3 unavailable. Download it again and refresh the page.'; });
  audio.addEventListener('ended', next);

  document.addEventListener('click', event => {
    const button = event.target.closest('button');
    if (!button) return;
    if (button.classList.contains('play-track')) {
      const row = button.closest('li');
      if (row?.dataset.playable === 'true') play({source: row.dataset.source, artist: row.dataset.artist, title: row.dataset.title});
    }
    if (button.id === 'player-prev') previous();
    if (button.id === 'player-next') next();
    if (button.id === 'player-shuffle') {
      const enabled = button.getAttribute('aria-pressed') !== 'true';
      setShuffle(enabled);
      try {
        if (enabled) localStorage.setItem(shuffleKey, 'true');
        else localStorage.removeItem(shuffleKey);
      } catch (_) {
        // Keep the selected state active for this page even without storage.
      }
    }
    if (button.id === 'player-mute') {
      if (audio.muted || audio.volume === 0) {
        audio.muted = false;
        if (audio.volume === 0) audio.volume = previousVolume;
      } else {
        previousVolume = audio.volume;
        audio.muted = true;
      }
      updateVolume();
    }
    if (button.id === 'player-toggle') {
      if (!audio.src) { next(); return; }
      if (audio.paused) audio.play().catch(() => { $('player-error').textContent = 'Could not play this MP3.'; });
      else audio.pause();
    }
  });
  document.addEventListener('input', event => {
    if (event.target.id === 'player-volume') {
      audio.volume = event.target.value / 100;
      audio.muted = false;
      if (audio.volume > 0) previousVolume = audio.volume;
      updateVolume();
    }
    if (event.target.id === 'player-seek' && Number.isFinite(audio.duration) && audio.duration > 0)
      audio.currentTime = audio.duration * event.target.value / 1000;
  });
  document.addEventListener('download:done', loadTracks);
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') loadTracks();
  });
  loadTracks();
  updateVolume();
})();
