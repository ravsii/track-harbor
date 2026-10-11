(() => {
  const audio = document.getElementById('player-audio');
  if (!audio) return;

  const $ = id => document.getElementById(id);
  const client = crypto.randomUUID(); // One owner per tab, even across browser profiles.
  const lastTrackKey = 'trackharbor-last-track';
  const stamp = track => `${track.source}\0${track.artist}\0${track.title}`;
  const time = seconds => Number.isFinite(seconds) ? `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}` : '0:00';
  let tracks = [], selected, shared, updatedAt = Date.now(), lastContact = 0;
  let previousVolume = 0.8, userMuted = false, releasing = false, pausing = false;
  audio.volume = 0.8;
  audio.muted = true;
  audio.preload = 'metadata';

  function icon(id, name) {
    $(id).querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${name}`);
  }

  function setShuffle(enabled) {
    $('player-shuffle').setAttribute('aria-pressed', String(enabled));
    $('player-shuffle').title = enabled ? 'Shuffle on' : 'Shuffle off';
  }

  function updateVolume() {
    const muted = userMuted || audio.volume === 0;
    icon('player-mute', muted ? 'volume-mute' : 'volume-high');
    $('player-mute').setAttribute('aria-label', muted ? 'Unmute' : 'Mute');
    $('player-mute').title = muted ? 'Unmute' : 'Mute';
    $('player-volume').value = muted ? 0 : Math.round(audio.volume * 100);
    audio.muted = muted || !ownsSound();
  }

  const ownsSound = () => !pausing && shared?.owner === client && shared.playing && !shared.pending && Date.now() - lastContact < 6000;
  const canPauseHere = () => shared?.playing && shared.owner === client && !shared.pending;
  const position = () => ownsSound() && !audio.paused ? audio.currentTime :
    Math.max(0, (shared?.position || 0) + (shared?.playing ? (Date.now() - updatedAt) / 1000 : 0));

  function refresh() {
    const remote = !!(shared?.playing && shared.owner && shared.owner !== client);
    $('player-remote').hidden = !remote;
    $('music-player').classList.toggle('remote-playing', remote);
    icon('player-toggle', canPauseHere() ? 'pause' : 'play');
    $('player-toggle').setAttribute('aria-label', canPauseHere() ? 'Pause' : shared?.playing ? 'Play here' : 'Play');
    const elapsed = position();
    $('player-time').textContent = `${time(elapsed)} / ${time(audio.duration)}`;
    $('player-seek').value = Number.isFinite(audio.duration) && audio.duration > 0
      ? Math.min(1000, Math.round(elapsed / audio.duration * 1000)) : 0;
  }

  function highlightCurrent() {
    document.querySelectorAll('.library:not(.download-library) .tracks li[data-source]').forEach(row => {
      const active = !!shared?.track && stamp(shared.track) === stamp(row.dataset);
      const playing = active && shared.playing;
      row.classList.toggle('current-track', active);
      row.classList.toggle('is-playing', playing);
      const button = row.querySelector('.play-track');
      const pause = playing && canPauseHere();
      button.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${pause ? 'pause' : 'play'}`);
      button.setAttribute('aria-label', `${pause ? 'Pause' : playing ? 'Play here' : 'Play'} ${row.dataset.title}`);
      if (active) row.setAttribute('aria-current', 'true');
      else row.removeAttribute('aria-current');
    });
  }

  function select(track) {
    if (selected && stamp(selected) === stamp(track)) return false;
    selected = track;
    const params = new URLSearchParams({source: track.source, artist: track.artist, title: track.title});
    audio.src = `/audio?${params}`;
    $('player-track').textContent = `${track.artist} — ${track.title}`;
    $('player-error').textContent = '';
    return true;
  }

  function chooseCandidate() {
    if (shared?.track || selected || !tracks.length) return;
    let remembered;
    try { remembered = JSON.parse(localStorage.getItem(lastTrackKey)); } catch (_) { /* Storage may be disabled. */ }
    select((remembered && tracks.find(track => stamp(track) === stamp(remembered))) || tracks[0]);
    refresh();
  }

  async function loadTracks() {
    try {
      const response = await fetch('/audio/tracks');
      if (!response.ok) throw new Error();
      tracks = await response.json();
      chooseCandidate();
    } catch (_) {
      $('player-error').textContent = 'Could not load saved tracks. Refresh the page to retry.';
    }
  }

  async function command(action, extra = {}) {
    const response = await fetch('/player/command', {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({client, action, ...extra})
    });
    if (!response.ok) throw new Error('player command failed');
  }

  function stopLocal() {
    audio.muted = true;
    audio.pause();
  }

  async function apply(state) {
    if (state.revision < (shared?.revision ?? -1)) return;
    lastContact = Date.now();
    const wasOwner = shared?.owner === client;
    const changed = !shared || state.revision !== shared.revision;
    shared = state;
    if (!state.playing || state.owner !== client) pausing = false;
    updatedAt = Date.now();
    setShuffle(state.shuffle);

    if (state.owner === client && state.pending && !releasing) {
      releasing = true;
      const position = audio.currentTime || 0;
      stopLocal(); // Stop sound before granting the new browser ownership.
      try { await command('release', {position}); } catch (_) { /* Lease expiry handles a lost acknowledgement. */ }
      releasing = false;
      refresh();
      return;
    }
    if (state.track) {
      const selectedChanged = select(state.track);
      if (selectedChanged || changed || !wasOwner && state.owner === client) {
        const seek = () => {
          if (audio.readyState && Number.isFinite(audio.duration) && shared?.revision === state.revision) {
            audio.currentTime = Math.min(state.position, audio.duration);
            if (ownsSound()) audio.muted = userMuted || audio.volume === 0;
          }
        };
        if (audio.readyState) seek();
        else audio.addEventListener('loadedmetadata', seek, {once: true});
      }
    } else {
      chooseCandidate();
    }
    highlightCurrent();
    if (ownsSound()) {
      // An existing owner must remain audible while loading the next track:
      // muted media in a background tab may not load until the tab is focused.
      // A new owner stays muted until its initial position has been restored.
      audio.muted = (!wasOwner && !audio.readyState) || userMuted || audio.volume === 0;
      if (audio.paused && state.track) {
        try {
          await audio.play();
          if (!ownsSound()) stopLocal();
        }
        catch (_) {
          stopLocal();
          if (ownsSound()) {
            $('player-error').textContent = 'Playback blocked. Click Play again in this browser.';
            try { await command('pause', {position: state.position}); } catch (_) { /* Retry via Play. */ }
          }
        }
      }
    } else {
      stopLocal();
    }
    refresh();
  }

  function prime(track) {
    if (!track) {
      $('player-error').textContent = 'Download a track before playing it.';
      return;
    }
    select(track);
    audio.muted = true; // Start inside the click gesture, but make no sound until the server grants ownership.
    audio.play().catch(() => { /* May require another click after handoff. */ });
    command('play', {track}).catch(() => {
      stopLocal();
      $('player-error').textContent = 'Could not start playback. Try again.';
    });
  }

  async function next() {
    if (!shared?.track && tracks.length) { prime(tracks[0]); return; }
    try { await command('next'); } catch (_) { $('player-error').textContent = 'Could not skip track.'; }
  }

  async function previous() {
    if (position() > 3) { seek(0); return; }
    try { await command('previous'); } catch (_) { $('player-error').textContent = 'Could not skip track.'; }
  }

  function seek(value) {
    if (ownsSound() && audio.readyState) audio.currentTime = value;
    command('seek', {position: value}).catch(() => { $('player-error').textContent = 'Could not seek.'; });
  }

  function pause() {
    const at = position();
    pausing = true;
    stopLocal();
    command('pause', {position: at}).catch(() => {
      pausing = false;
      $('player-error').textContent = 'Could not pause playback.';
    });
  }

  audio.addEventListener('timeupdate', refresh);
  audio.addEventListener('durationchange', refresh);
  audio.addEventListener('loadedmetadata', refresh);
  audio.addEventListener('pause', refresh);
  audio.addEventListener('volumechange', updateVolume);
  audio.addEventListener('playing', () => {
    if (!ownsSound() || !selected) return;
    try { localStorage.setItem(lastTrackKey, JSON.stringify(selected)); } catch (_) { /* Storage may be disabled. */ }
    refresh();
  });
  audio.addEventListener('error', () => {
    stopLocal();
    $('player-error').textContent = 'MP3 unavailable. Download it again and refresh the page.';
    if (shared?.owner === client) command('pause', {position: 0}).catch(() => {});
  });
  audio.addEventListener('ended', () => {
    if (shared?.owner === client) next();
  });

  document.addEventListener('click', event => {
    const button = event.target.closest('button');
    if (!button) return;
    if (button.classList.contains('play-track')) {
      const row = button.closest('li');
      if (row?.dataset.playable === 'true') {
        if (shared?.track && stamp(shared.track) === stamp(row.dataset) && canPauseHere())
          pause();
        else prime({source: row.dataset.source, artist: row.dataset.artist, title: row.dataset.title});
      }
    }
    if (button.id === 'player-prev') previous();
    if (button.id === 'player-next') next();
    if (button.id === 'player-shuffle') command('shuffle', {shuffle: shuffleButton.getAttribute('aria-pressed') !== 'true'}).catch(() => {});
    if (button.id === 'player-mute') {
      if (userMuted || audio.volume === 0) {
        userMuted = false;
        if (audio.volume === 0) audio.volume = previousVolume;
      } else {
        previousVolume = audio.volume;
        userMuted = true;
      }
      updateVolume();
    }
    if (button.id === 'player-toggle') {
      if (canPauseHere()) pause();
      else prime(shared?.track || selected || tracks[0]);
    }
  });
  const shuffleButton = $('player-shuffle');
  document.addEventListener('input', event => {
    if (event.target.id === 'player-volume') {
      audio.volume = event.target.value / 100;
      userMuted = false;
      if (audio.volume > 0) previousVolume = audio.volume;
      updateVolume();
    }
    if (event.target.id === 'player-seek' && Number.isFinite(audio.duration) && audio.duration > 0)
      seek(audio.duration * event.target.value / 1000);
  });
  document.addEventListener('download:done', loadTracks);
  document.addEventListener('download:missing', loadTracks);
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') {
      highlightCurrent();
      loadTracks();
    }
  });

  const events = new EventSource('/player/events');
  events.onmessage = async event => {
    try {
      const state = JSON.parse(event.data);
      await apply(state);
      if (ownsSound() && !audio.paused) {
        await command('report', {position: audio.currentTime, revision: shared.revision});
      }
    } catch (_) { stopLocal(); }
  };
  events.onerror = stopLocal;
  setInterval(() => {
    if (Date.now() - lastContact > 6000) stopLocal();
    refresh();
  }, 1000);
  window.addEventListener('pagehide', () => {
    if (shared?.owner === client) {
      stopLocal();
      navigator.sendBeacon('/player/command', new Blob([JSON.stringify({client, action: 'release', position: audio.currentTime || 0})], {type: 'application/json'}));
    }
  });
  fetch('/player/state').then(response => response.json()).then(apply).catch(stopLocal);
  loadTracks();
  updateVolume();
})();
