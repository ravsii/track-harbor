(() => {
  const audio = document.getElementById('player-audio');
  if (!audio) return;
  const $ = id => document.getElementById(id);
  const cover = $('player-cover-image');
  const rememberedKey = `trackharbor-last-track:${document.body.dataset.userId}`;
  let tracks = [], selected, started = false, shuffle = false, muted = false, previousVolume = 0.8;
  const history = [];
  audio.volume = 0.8;
  audio.preload = 'metadata';
  const time = seconds => Number.isFinite(seconds) ? `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}` : '0:00';
  const icon = (id, name) => $(id).querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${name}`);
  cover.addEventListener('load', () => { cover.hidden = false; });
  cover.addEventListener('error', () => { cover.hidden = true; });
  document.addEventListener('error', event => {
    if (event.target.matches?.('.library .track-cover img')) event.target.hidden = true;
  }, true);

  function highlight() {
    document.querySelectorAll('.library:not(.download-library) li[data-id]').forEach(row => {
      const active = started && String(selected?.id) === row.dataset.id;
      const playing = active && !audio.paused;
      row.classList.toggle('current-track', active);
      row.classList.toggle('is-playing', playing);
      const button = row.querySelector('.play-track');
      button.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${playing ? 'pause' : 'play'}`);
      button.setAttribute('aria-label', `${playing ? 'Pause' : 'Play'} ${row.dataset.title}`);
      button.title = button.getAttribute('aria-label');
      if (active) row.setAttribute('aria-current', 'true');
      else row.removeAttribute('aria-current');
    });
  }
  function refresh() {
    icon('player-toggle', audio.paused ? 'play' : 'pause');
    $('player-toggle').setAttribute('aria-label', audio.paused ? 'Play' : 'Pause');
    $('player-time').textContent = `${time(audio.currentTime)} / ${time(audio.duration)}`;
    $('player-seek').value = Number.isFinite(audio.duration) && audio.duration > 0 ? Math.min(1000, Math.round(audio.currentTime / audio.duration * 1000)) : 0;
  }
  function select(track) {
    const next = tracks.find(item => String(item.id) === String(track.id)) || track;
    const image = next.cover || '';
    if ((cover.getAttribute('src') || '') !== image) {
      cover.hidden = true;
      if (image) cover.src = image;
      else cover.removeAttribute('src');
    }
    $('player-track').textContent = `${next.artist} — ${next.title}`;
    if (selected?.id === next.id) { selected = next; return; }
    selected = next;
    audio.src = `/audio?id=${encodeURIComponent(next.id)}`;
    $('player-error').textContent = '';
    try { sessionStorage.setItem(rememberedKey, String(next.id)); } catch (_) { /* Storage may be disabled. */ }
  }
  async function play(track = selected) {
    if (!track) { $('player-error').textContent = 'Download a track before playing it.'; return; }
    select(track);
    started = true;
    try { await audio.play(); }
    catch (_) { $('player-error').textContent = 'Could not play this track. Click Play to retry.'; }
    highlight(); refresh();
  }
  function next() {
    if (!tracks.length) return;
    if (selected) history.push(selected.id);
    if (history.length > 100) history.shift();
    const index = tracks.findIndex(track => track.id === selected?.id);
    let choice = (index + 1) % tracks.length;
    if (shuffle && tracks.length > 1) {
      choice = Math.floor(Math.random() * (tracks.length - 1));
      if (choice >= index && index >= 0) choice++;
    }
    if (tracks[choice].id === selected?.id) audio.currentTime = 0;
    play(tracks[choice]);
  }
  function previous() {
    if (audio.currentTime > 3) { audio.currentTime = 0; return; }
    let track;
    while (history.length && !track) track = tracks.find(item => item.id === history.pop());
    if (!track && tracks.length) {
      const index = tracks.findIndex(item => item.id === selected?.id);
      track = tracks[(index - 1 + tracks.length) % tracks.length];
    }
    if (track) play(track);
  }
  function volume() {
    audio.muted = muted || audio.volume === 0;
    icon('player-mute', audio.muted ? 'volume-mute' : 'volume-high');
    $('player-mute').setAttribute('aria-label', audio.muted ? 'Unmute' : 'Mute');
    $('player-mute').title = audio.muted ? 'Unmute' : 'Mute';
    $('player-volume').value = audio.muted ? 0 : Math.round(audio.volume * 100);
  }
  function applyTracks(items) {
    const selectedFromQueue = tracks.some(track => track.id === selected?.id);
    tracks = items;
    const updated = tracks.find(track => track.id === selected?.id);
    if (updated) select(updated); // Metadata refreshes never reset the audio source.
    // Explicit catalog playback is independent of the personal automatic queue.
    else if (!selected || !started || selectedFromQueue) {
      audio.pause();
      started = false;
      selected = undefined;
      if (tracks.length) {
        let remembered;
        try { remembered = sessionStorage.getItem(rememberedKey); } catch (_) { /* Storage may be disabled. */ }
        select(tracks.find(track => String(track.id) === remembered) || tracks[0]);
      } else {
        audio.removeAttribute('src');
        audio.load();
        $('player-track').textContent = '';
        cover.removeAttribute('src');
        cover.hidden = true;
      }
    }
    refresh(); highlight();
  }
  let catalogueRequest;
  async function loadTracks() {
    if (catalogueRequest) return catalogueRequest;
    catalogueRequest = fetchCatalogue();
    try { await catalogueRequest; } finally { catalogueRequest = undefined; }
  }
  async function fetchCatalogue() {
    try {
      const response = await fetch('/audio/tracks');
      if (!response.ok) throw new Error();
      applyTracks(await response.json());
    } catch (_) { $('player-error').textContent = 'Could not load saved tracks. Refresh the page to retry.'; }
  }
  document.addEventListener('click', event => {
    const button = event.target.closest('button');
    if (!button || button.disabled) return;
    if (button.classList.contains('play-track')) {
      const row = button.closest('[data-id]');
      if (row.dataset.playable !== 'true') return;
      if (started && String(selected?.id) === row.dataset.id && !audio.paused) audio.pause();
      else play(tracks.find(track => String(track.id) === row.dataset.id) || {id: Number(row.dataset.id), artist: row.dataset.artist, title: row.dataset.title});
    }
    if (button.id === 'player-toggle') { if (audio.paused) play(); else audio.pause(); }
    if (button.id === 'player-next') next();
    if (button.id === 'player-prev') previous();
    if (button.id === 'player-shuffle') {
      shuffle = !shuffle;
      button.setAttribute('aria-pressed', String(shuffle));
      button.title = shuffle ? 'Shuffle on' : 'Shuffle off';
    }
    if (button.id === 'player-mute') {
      if (audio.volume === 0) { audio.volume = previousVolume || 0.8; muted = false; }
      else muted = !muted;
      volume();
    }
  });
  $('player-volume').addEventListener('input', event => {
    audio.volume = Number(event.target.value) / 100;
    if (audio.volume > 0) previousVolume = audio.volume;
    muted = false; volume();
  });
  $('player-seek').addEventListener('input', event => {
    if (Number.isFinite(audio.duration)) audio.currentTime = Number(event.target.value) / 1000 * audio.duration;
  });
  for (const name of ['play', 'pause']) audio.addEventListener(name, () => { refresh(); highlight(); });
  for (const name of ['timeupdate', 'durationchange']) audio.addEventListener(name, refresh);
  audio.addEventListener('ended', next);
  audio.addEventListener('error', () => { $('player-error').textContent = 'Audio unavailable. Check Downloads.'; });
  document.addEventListener('htmx:afterSwap', event => { if (event.detail.target.id === 'content') { highlight(); loadTracks(); } });
  document.addEventListener('download:catalog', event => applyTracks(event.detail));
  document.addEventListener('download:missing', event => {
    if (String(selected?.id) === String(event.detail?.id)) audio.pause();
  });
  document.addEventListener('auth:expired', () => { audio.pause(); audio.removeAttribute('src'); audio.load(); });
  volume(); refresh(); loadTracks();
})();
