(() => {
  const audio = document.getElementById('player-audio');
  if (!audio) return;

  const $ = id => document.getElementById(id);
  const stamp = track => `${track.source}\0${track.artist}\0${track.title}`;
  const time = seconds => Number.isFinite(seconds) ? `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}` : '0:00';
  let tracks = [], current, history = [];
  audio.volume = 0.8;

  function refresh() {
    $('player-toggle').textContent = audio.paused ? '▶' : 'Ⅱ';
    $('player-toggle').setAttribute('aria-label', audio.paused ? 'Play' : 'Pause');
    $('player-time').textContent = `${time(audio.currentTime)} / ${time(audio.duration)}`;
    $('player-seek').value = Number.isFinite(audio.duration) && audio.duration > 0
      ? Math.round(audio.currentTime / audio.duration * 1000) : 0;
  }

  function updateChoices() {
    const selector = $('player-track-choice');
    selector.replaceChildren(new Option('Saved tracks', ''));
    for (const [index, track] of tracks.entries()) {
      selector.add(new Option(`${track.artist} — ${track.title}`, String(index)));
    }
    const selected = tracks.findIndex(track => stamp(track) === current);
    selector.value = selected < 0 ? '' : String(selected);
  }

  async function loadTracks() {
    try {
      const response = await fetch('/audio/tracks');
      if (!response.ok) throw new Error('unavailable');
      tracks = await response.json();
      updateChoices();
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
    const selected = tracks.findIndex(item => stamp(item) === current);
    $('player-track-choice').value = selected < 0 ? '' : String(selected);
    try { await audio.play(); } catch (_) { $('player-error').textContent = 'Could not play this MP3. Check that it is still downloaded.'; }
    refresh();
  }

  async function next() {
    if (!tracks.length) await loadTracks();
    if (!tracks.length) { await play(null); return; }
    const index = tracks.findIndex(track => stamp(track) === current);
    if ($('player-shuffle').checked && tracks.length > 1) {
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
  audio.addEventListener('error', () => { $('player-error').textContent = 'MP3 unavailable. Download it again and refresh the page.'; });
  audio.addEventListener('ended', () => {
    if ($('player-repeat').checked) { audio.currentTime = 0; audio.play().catch(() => {}); }
    else next();
  });

  document.addEventListener('click', event => {
    const button = event.target.closest('button');
    if (!button) return;
    if (button.classList.contains('play-track')) {
      const row = button.closest('li');
      if (row?.dataset.playable === 'true') play({source: row.dataset.source, artist: row.dataset.artist, title: row.dataset.title});
    }
    if (button.id === 'player-prev') previous();
    if (button.id === 'player-next') next();
    if (button.id === 'player-toggle') {
      if (!audio.src) { next(); return; }
      if (audio.paused) audio.play().catch(() => { $('player-error').textContent = 'Could not play this MP3.'; });
      else audio.pause();
    }
  });
  $('player-track-choice').addEventListener('change', event => {
    const track = tracks[Number(event.target.value)];
    if (track) play(track);
  });
  document.addEventListener('input', event => {
    if (event.target.id === 'player-volume') audio.volume = event.target.value / 100;
    if (event.target.id === 'player-seek' && Number.isFinite(audio.duration) && audio.duration > 0)
      audio.currentTime = audio.duration * event.target.value / 1000;
  });
  document.addEventListener('download:done', loadTracks);
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') loadTracks();
  });
  loadTracks();
})();
