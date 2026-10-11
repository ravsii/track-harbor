(() => {
  let audio, current, history = [];
  const $ = id => document.getElementById(id);
  const rows = () => [...document.querySelectorAll('.music-library .tracks li[data-playable="true"]')];
  const stamp = row => `${row.dataset.source}\0${row.dataset.artist}\0${row.dataset.title}`;
  const time = seconds => Number.isFinite(seconds) ? `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}` : '0:00';

  function refresh() {
    if (!audio || !$('player-toggle')) return;
    $('player-toggle').textContent = audio.paused ? '▶' : 'Ⅱ';
    $('player-time').textContent = `${time(audio.currentTime)} / ${time(audio.duration)}`;
    $('player-seek').value = Number.isFinite(audio.duration) && audio.duration > 0
      ? Math.round(audio.currentTime / audio.duration * 1000) : 0;
  }

  async function play(row, remember = true) {
    if (!audio || !row || row.dataset.playable !== 'true') return;
    if (remember && current && current !== stamp(row)) history.push(current);
    current = stamp(row);
    const params = new URLSearchParams({source: row.dataset.source, artist: row.dataset.artist, title: row.dataset.title});
    audio.src = `/audio?${params}`;
    $('player-track').textContent = `${row.dataset.artist} — ${row.dataset.title}`;
    $('player-error').textContent = '';
    try { await audio.play(); } catch (_) { $('player-error').textContent = 'Could not play this MP3. Check that it is still downloaded.'; }
    refresh();
  }

  function next() {
    const list = rows();
    if (!list.length) return;
    const index = list.findIndex(row => stamp(row) === current);
    if ($('player-shuffle').checked && list.length > 1) {
      const candidates = list.filter(row => stamp(row) !== current);
      play(candidates[Math.floor(Math.random() * candidates.length)]);
    } else {
      play(list[(index + 1) % list.length]);
    }
  }

  function previous() {
    if (!audio) return;
    if (audio.currentTime > 3) { audio.currentTime = 0; return; }
    const list = rows();
    if (!list.length) return;
    const prior = history.pop();
    const index = list.findIndex(row => stamp(row) === current);
    play((prior && list.find(row => stamp(row) === prior)) || list[(index - 1 + list.length) % list.length], false);
  }

  function init() {
    audio = $('player-audio');
    current = null;
    history = [];
    if (!audio) return;
    audio.volume = 0.8;
    audio.addEventListener('timeupdate', refresh);
    audio.addEventListener('durationchange', refresh);
    audio.addEventListener('play', refresh);
    audio.addEventListener('pause', refresh);
    audio.addEventListener('error', () => { if ($('player-error')) $('player-error').textContent = 'MP3 unavailable. Download it again and refresh the page.'; });
    audio.addEventListener('ended', () => {
      if ($('player-repeat').checked) { audio.currentTime = 0; audio.play().catch(() => {}); }
      else next();
    });
  }

  document.addEventListener('click', event => {
    const button = event.target.closest('button');
    if (!button || !audio) return;
    if (button.classList.contains('play-track')) play(button.closest('li'));
    if (button.id === 'player-prev') previous();
    if (button.id === 'player-next') next();
    if (button.id === 'player-toggle') {
      if (!audio.src) { play(rows()[0]); return; }
      if (audio.paused) audio.play().catch(() => { $('player-error').textContent = 'Could not play this MP3.'; });
      else audio.pause();
    }
  });
  document.addEventListener('input', event => {
    if (!audio) return;
    if (event.target.id === 'player-volume') audio.volume = event.target.value / 100;
    if (event.target.id === 'player-seek' && Number.isFinite(audio.duration) && audio.duration > 0)
      audio.currentTime = audio.duration * event.target.value / 1000;
  });
  document.addEventListener('htmx:beforeSwap', event => {
    if (event.detail.target.id === 'content' && audio) { audio.pause(); audio.removeAttribute('src'); audio.load(); audio = null; }
  });
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') init();
  });
  init();
})();
