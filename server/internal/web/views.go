package web

import (
	"html/template"
	"time"
)

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"addedDate": func(timestamp int64) string {
		if timestamp == 0 {
			return "—"
		}
		return time.Unix(timestamp, 0).UTC().Format("02 Jan 2006, 15:04 UTC")
	},
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TrackHarbor · {{.Name}}</title>
  <link rel="stylesheet" href="/assets/app.css">
	  <link rel="stylesheet" href="/assets/theme.css">
	  <script src="/assets/theme.js"></script>
	  <script src="/assets/htmx.min.js" defer></script>
	  <script src="/assets/download.js" defer></script>
	  <script src="/assets/player.js" defer></script>
</head>
<body>
<div class="shell">
  <aside class="sidebar">
    <div class="brand"><span class="mark">◉</span><div><strong>TrackHarbor</strong><small>Your music, your harbor.</small></div></div>
    <div class="nav-label">YOUR LIBRARY</div>
    <nav aria-label="Sources">
      <a class="{{if eq .Source "music"}}active{{end}}" href="/" hx-get="/" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>◉</span> Music <span class="arrow">↗</span></a>
      <a class="{{if eq .Source "spotify"}}active{{end}}" href="/?source=spotify" hx-get="/?source=spotify" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>◈</span> Spotify <span class="arrow">↗</span></a>
      <a class="{{if eq .Source "vk"}}active{{end}}" href="/?source=vk" hx-get="/?source=vk" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>♫</span> VK Music <span class="arrow">↗</span></a>
    </nav>
    <label class="theme-control" for="theme-choice">THEME
      <select id="theme-choice" aria-label="Color theme">
        <option value="system">As in system</option>
        <option value="light">Light</option>
        <option value="dark">Dark</option>
      </select>
    </label>
    <div class="sidebar-bottom"><span class="dot"></span> LOCAL SERVER <p>This preview runs on your computer only. No web login is configured.</p></div>
  </aside>
  <header id="music-player" class="panel player topbar" aria-label="Music player">
    <div class="topbar-track">
      <span class="index">NOW PLAYING / LOCAL FILES</span>
      <strong id="player-track" aria-live="polite">Choose a downloaded track</strong>
      <select id="player-track-choice" aria-label="Choose a downloaded track"><option value="">Saved tracks</option></select>
    </div>
    <audio id="player-audio" preload="none"></audio>
    <div class="player-controls">
      <button type="button" id="player-prev" class="secondary" aria-label="Previous track">◀◀</button>
      <button type="button" id="player-toggle" class="secondary" aria-label="Play or pause">▶</button>
      <button type="button" id="player-next" class="secondary" aria-label="Next track">▶▶</button>
      <label>Position <input type="range" id="player-seek" min="0" max="1000" value="0" aria-label="Seek"></label>
      <span id="player-time">0:00 / 0:00</span>
      <label>Volume <input type="range" id="player-volume" min="0" max="100" value="80" aria-label="Volume"></label>
      <label><input type="checkbox" id="player-shuffle"> Shuffle</label>
      <label><input type="checkbox" id="player-repeat"> Repeat one</label>
    </div>
    <div id="player-error" role="status" aria-live="polite"></div>
  </header>
  <main id="content" data-source="{{.Source}}" hx-history-elt>
    <div class="topline"><span>LIBRARY / {{.Name}}</span><span class="label">EARLY ACCESS</span></div>
    <header class="hero"><div class="eyebrow">YOUR COLLECTION, IN ONE PLACE</div><h1>{{.Name}}</h1><p>{{if eq .Source "music"}}All your tracks from Spotify and VK Music, together and ordered by when they were added.{{else}}Connect your account, sync your library, and explore what you love. Each source has its own collection.{{end}}</p></header>
    {{if ne .Source "music"}}
    <section class="grid">
      <div class="panel connect"><div class="panel-top"><span class="index">01 / CONNECTION</span><span class="pill {{if .Connected}}connected{{end}}">{{if .Connected}}● Connected{{else}}○ Not connected{{end}}</span></div>
        <h2>Connect {{.Name}}</h2>
        {{if eq .Source "spotify"}}
        <p>Paste the full request headers from a Spotify Web Player request. Only <code>authorization</code> and <code>client-token</code> are saved; other headers and cookies are discarded.</p>
        <form action="/connect" method="post" autocomplete="off"><input type="hidden" name="source" value="spotify">
          <label>REQUEST HEADERS<textarea name="headers" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="authorization: Bearer …&#10;client-token: …" required></textarea></label>
          <button type="submit">Save credentials <span>↗</span></button>
        </form>
        {{else}}
        <p><a href="{{.AuthURL}}" target="_blank" rel="noopener noreferrer">Open VK authorization ↗</a>, then paste the token or full redirect URL below. The redirect fragment stays in your browser until you paste it.</p>
        <form action="/connect" method="post" autocomplete="off"><input type="hidden" name="source" value="vk">
          <label>ACCESS TOKEN OR REDIRECT URL<input name="token" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Paste token or redirect URL" required></label>
          <button type="submit">Save credentials <span>↗</span></button>
        </form>
        {{end}}
        <form action="/ping" method="post"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="secondary" {{if not .Connected}}disabled{{end}}>Check connection <span>→</span></button></form>
        <div class="status" aria-live="polite">CONNECTION <span hx-get="/ping-status?source={{.Source}}" hx-trigger="every 3s" hx-swap="innerHTML">{{if .PingStatus}}{{.PingStatus}}{{else}}Not checked yet.{{end}}</span></div>
      </div>
      <div class="panel sync"><div class="panel-top"><span class="index">02 / SYNC</span><span class="mini-mark">↻</span></div>
        <h2>Bring your tracks in</h2>
        <p>Fetch your latest liked tracks. Your current list stays available if a sync fails.</p>
        <form action="/sync" method="post"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="secondary" {{if not .Connected}}disabled{{end}}>Sync library <span>→</span></button></form>
        <div class="status" aria-live="polite">STATUS <span id="sync-state" hx-get="/status?source={{.Source}}" hx-trigger="every 3s" hx-swap="innerHTML">{{if .Status}}{{.Status}}{{else}}Ready to sync.{{end}}</span></div>
      </div>
    </section>
    {{end}}
    <section class="panel download-panel"><div class="panel-top"><span class="index">DOWNLOADS / MANUAL START</span></div>
      <h2>Save tracks locally</h2><p>Downloads start only when you press a button. Existing MP3s are skipped.</p>
      <form action="/download" method="post"><input type="hidden" name="mode" value="all"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" {{if not .Tracks}}disabled{{end}}>Download missing tracks <span>→</span></button></form>
      <div id="download-state" class="status" role="status" aria-live="polite">Ready to download.</div>
    </section>
    <dialog id="download-dialog" aria-labelledby="download-dialog-message"><p id="download-dialog-message"></p><button type="button" id="download-dialog-close">OK</button></dialog>
    <section class="library {{if eq .Source "music"}}music-library{{end}}"><div class="library-heading"><div><span class="index">{{if eq .Source "music"}}01 / ALL MUSIC{{else}}03 / COLLECTION{{end}}</span><h2>{{if eq .Source "music"}}All tracks{{else}}Your tracks{{end}}</h2></div><span class="count">{{len .Tracks}} TRACKS</span></div>
      {{if .Tracks}}
        <div class="track-head"><span>#</span><span>TRACK</span><span>ARTIST</span>{{if eq .Source "music"}}<span>SOURCE</span><span>ADDED</span>{{end}}<span>FILE</span></div>
        <ol class="tracks">{{range $i, $t := .Tracks}}<li data-source="{{$t.Source}}" data-artist="{{$t.Artist}}" data-title="{{$t.Title}}" data-playable="{{if $t.FilePath}}true{{else}}false{{end}}"><span class="number">{{add $i 1}}</span><strong>{{$t.Title}}</strong><span class="artist">{{if $t.Artist}}{{$t.Artist}}{{else}}Unknown artist{{end}}</span>{{if eq $.Source "music"}}<span class="source-tag {{if eq $t.Source "spotify"}}spotify{{else}}vk{{end}}">{{if eq $t.Source "spotify"}}Spotify{{else}}VK Music{{end}}</span><time class="added-date">{{addedDate $t.AddedAt}}</time>{{end}}<div class="track-actions"><button type="button" class="play-track secondary" {{if not $t.FilePath}}disabled{{end}} aria-label="Play {{$t.Title}}">▶</button><form action="/download" method="post"><input type="hidden" name="mode" value="one"><input type="hidden" name="view" value="{{$.Source}}"><input type="hidden" name="source" value="{{$t.Source}}"><input type="hidden" name="artist" value="{{$t.Artist}}"><input type="hidden" name="title" value="{{$t.Title}}"><button class="secondary" type="submit">{{if $t.FilePath}}Saved{{else}}Download{{end}}</button></form></div></li>{{end}}</ol>
      {{else}}
        <div class="empty"><span>◌</span><h3>Nothing here yet</h3><p>{{if eq .Source "music"}}Connect and sync Spotify or VK Music to fill your library.{{else if .Connected}}Run your first sync to see tracks from {{.Name}}.{{else}}Connect {{.Name}} above, then sync your library.{{end}}</p></div>
      {{end}}
    </section>
    <footer>LOCAL DOWNLOADS · TRACKHARBOR</footer>
  </main>
</div>
</body></html>`))
