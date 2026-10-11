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
	  <script src="/assets/settings.js" defer></script>
	  <script src="/assets/player.js" defer></script>
	  <script src="/assets/duration.js" defer></script>
</head>
<body>
<div class="shell">
  <header id="music-player" class="topbar" aria-label="Music player">
    <a class="brand" href="/" hx-get="/" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true" aria-label="TrackHarbor home"><span class="mark">{{template "icon" "disc-album"}}</span><strong>TrackHarbor</strong></a>
    <div class="topbar-content">
      <audio id="player-audio" preload="none"></audio>
      <div class="player-controls">
        <button type="button" id="player-shuffle" aria-label="Shuffle" aria-pressed="false" title="Shuffle off">{{template "icon" "shuffle"}}</button>
        <button type="button" id="player-prev" aria-label="Previous track">{{template "icon" "previous"}}</button>
        <button type="button" id="player-toggle" class="primary" aria-label="Play">{{template "icon" "play"}}</button>
        <button type="button" id="player-next" aria-label="Next track">{{template "icon" "next"}}</button>
        <div class="player-progress">
          <strong id="player-track" aria-live="polite"></strong>
          <div class="player-progress-row">
            <input type="range" id="player-seek" min="0" max="1000" value="0" aria-label="Seek">
            <span id="player-time">0:00 / 0:00</span>
          </div>
        </div>
        <button type="button" id="player-mute" aria-label="Mute" title="Mute">{{template "icon" "volume-high"}}</button>
        <input type="range" id="player-volume" min="0" max="100" value="80" aria-label="Volume">
      </div>
	      <div class="topbar-actions">
	        <a id="system-status" href="/?source=downloads" hx-get="/?source=downloads" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true" title="Open Downloads"><span class="system-indicator" aria-hidden="true"><span class="system-waves"></span><span class="system-dot"></span><span class="system-sync-icon">{{template "icon" "reload"}}</span></span><span id="system-status-text">Checking status…</span></a>
	        <button type="button" id="theme-choice" class="theme-toggle" aria-label="Theme: system" title="Theme: system">{{template "icon" "computer"}}</button>
	      </div>
      <div id="player-error" role="status" aria-live="polite"></div>
    </div>
  </header>
  <aside class="sidebar">
	    <div class="nav-label">NAVIGATION</div>
	    <nav aria-label="Navigation">
      <a class="{{if eq .Source "music"}}active{{end}}" href="/" hx-get="/" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>{{template "icon" "music-note-01"}}</span> Music <span class="arrow">{{template "icon" "arrow-right-01"}}</span></a>
      <a class="{{if eq .Source "spotify"}}active{{end}}" href="/?source=spotify" hx-get="/?source=spotify" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>{{template "icon" "spotify"}}</span> Spotify <span class="arrow">{{template "icon" "arrow-right-01"}}</span></a>
	      <a class="{{if eq .Source "vk"}}active{{end}}" href="/?source=vk" hx-get="/?source=vk" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>{{template "icon" "vk"}}</span> VK Music <span class="arrow">{{template "icon" "arrow-right-01"}}</span></a>
	      <a class="{{if eq .Source "downloads"}}active{{end}}" href="/?source=downloads" hx-get="/?source=downloads" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>{{template "icon" "download-01"}}</span> Downloads <span class="arrow">{{template "icon" "arrow-right-01"}}</span></a>
	      <a class="{{if eq .Source "settings"}}active{{end}}" href="/?source=settings" hx-get="/?source=settings" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>{{template "icon" "computer"}}</span> Settings <span class="arrow">{{template "icon" "arrow-right-01"}}</span></a>
    </nav>
  </aside>
	  <main id="content" data-source="{{.Source}}" hx-history-elt>
	    {{if ne .Source "music"}}
	    <header class="page-heading"><h1>{{.Name}}</h1></header>
	    {{end}}
	    {{if or (eq .Source "spotify") (eq .Source "vk")}}
    <section class="grid">
        <div class="panel connect"><div class="panel-top"><span class="index">01 / CONNECTION</span><span class="pill {{if .Connected}}connected{{end}}">{{if .Connected}}{{template "icon" "checkmark-circle-02"}} Connected{{else}}{{template "icon" "circle"}} Not connected{{end}}</span></div>
	        <h2>{{if .Connected}}Connection{{else}}Connect {{.Name}}{{end}}</h2>
	        {{if .Connected}}<details class="credential-details"><summary>Update credentials</summary>{{end}}
	        {{if eq .Source "spotify"}}
        <p>Paste the full request headers from a Spotify Web Player request. Only <code>authorization</code> and <code>client-token</code> are saved; other headers and cookies are discarded.</p>
        <form action="/connect" method="post" autocomplete="off"><input type="hidden" name="source" value="spotify">
          <label>REQUEST HEADERS<textarea name="headers" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="authorization: Bearer …&#10;client-token: …" required></textarea></label>
          <button type="submit">Save credentials <span>{{template "icon" "arrow-right-01"}}</span></button>
        </form>
        {{else}}
        <p><a href="{{.AuthURL}}" target="_blank" rel="noopener noreferrer">Open VK authorization {{template "icon" "external-link"}}</a>, then paste the token or full redirect URL below. The redirect fragment stays in your browser until you paste it.</p>
        <form action="/connect" method="post" autocomplete="off"><input type="hidden" name="source" value="vk">
          <label>ACCESS TOKEN OR REDIRECT URL<input name="token" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Paste token or redirect URL" required></label>
          <button type="submit">Save credentials <span>{{template "icon" "arrow-right-01"}}</span></button>
        </form>
	        {{end}}
	        {{if .Connected}}</details>{{end}}
	        <form action="/ping" method="post"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="secondary" {{if not .Connected}}disabled{{end}}>Check connection <span>{{template "icon" "arrow-right-01"}}</span></button></form>
        <div class="status" aria-live="polite">CONNECTION <span hx-get="/ping-status?source={{.Source}}" hx-trigger="every 3s" hx-swap="innerHTML">{{if .PingStatus}}{{.PingStatus}}{{else}}Not checked yet.{{end}}</span></div>
      </div>
      <div class="panel sync"><div class="panel-top"><span class="index">02 / SYNC</span><span class="mini-mark">{{template "icon" "reload"}}</span></div>
        <h2>Bring your tracks in</h2>
	        <p>Fetch your latest liked tracks, then check and download missing audio in the background. Your current list stays available if a sync fails.</p>
        <form action="/sync" method="post"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="secondary" {{if not .Connected}}disabled{{end}}>Sync library <span>{{template "icon" "arrow-right-01"}}</span></button></form>
        <div class="status" aria-live="polite">STATUS <span id="sync-state" hx-get="/status?source={{.Source}}" hx-trigger="every 10s" hx-swap="innerHTML">{{if .Status}}{{.Status}}{{else}}Ready to sync.{{end}}</span></div>
      </div>
    </section>
    {{end}}
	    {{if eq .Source "settings"}}
	    <section class="panel settings-panel">
	      <p>Choose the connection routes for new download batches. Downloads already running keep their current routes.</p>
	      <form id="settings-form" action="/settings" method="post">
	        <label>SPOTIFY / YOUTUBE ROUTE ORDER<textarea name="strategy" required spellcheck="false">{{.SettingsStrategy}}</textarea></label>
	        <p>One per line, in priority order: <code>direct</code>, <code>personal_proxy</code>, <code>random_proxy</code>.</p>
	        <div class="settings-fields">
	          <div class="proxy-field"><label>PERSONAL PROXIES<textarea name="personal_proxy" spellcheck="false" autocomplete="off" placeholder="One proxy URL per line. Leave empty to keep saved values."></textarea></label><p>{{.PersonalProxyCount}} saved. Addresses are never shown again; enter new URLs to replace the entire list.</p><label class="clear-option"><input type="checkbox" name="clear_personal_proxy"> Remove saved personal proxies</label></div>
	          <div class="proxy-field"><label>PUBLIC PROXIES<textarea name="public_proxy" spellcheck="false" autocomplete="off" placeholder="One proxy URL per line. Leave empty to keep saved values."></textarea></label><p>{{.PublicProxyCount}} saved. Addresses are never shown again; enter new URLs to replace the entire list.</p><label class="clear-option"><input type="checkbox" name="clear_public_proxy"> Remove saved public proxies</label></div>
	        </div>
	        <p>VK always uses <code>direct</code> (the only supported route). Set <code>listen</code> and <code>download_dir</code> in <code>config.yaml</code>; changing either requires a restart.</p>
	        <button type="submit">Save settings <span>{{template "icon" "arrow-right-01"}}</span></button>
	      </form>
	      <div id="settings-feedback" class="status" role="status" aria-live="polite">{{if .SettingsSaved}}Settings saved. New download batches will use them.{{end}}</div>
	    </section>
	    {{else if eq .Source "downloads"}}
	    <section class="panel download-panel"><div class="panel-top"><span class="index">DOWNLOAD CONTROL</span></div>
	      <button type="button" id="download-toggle" disabled><span class="download-toggle-label">Start downloads</span>{{template "icon" "play"}}</button>
	      <div id="download-state" class="status" role="status" aria-live="polite">Loading download status…</div>
	    </section>
	    <section class="library download-library"><div class="library-heading"><div><span class="index">TRACKS</span><h2>Download queue</h2></div><span class="count" id="download-count"></span></div>
	      <div class="track-head"><span>TRACK</span><span>ARTIST</span><span>SOURCE</span><span>STATUS</span></div>
	      <ol class="tracks" id="download-tracks"></ol>
	    </section>
	    {{else}}
	    <section class="library {{if eq .Source "music"}}music-library{{end}}"{{if eq .Source "music"}} aria-label="Tracks"{{end}}>{{if ne .Source "music"}}<div class="library-heading"><div><span class="index">03 / COLLECTION</span><h2>Your tracks</h2></div><span class="count">{{len .Tracks}} TRACKS</span></div>{{end}}
      {{if .Tracks}}
	        <div class="track-head"><span>#</span><span>TRACK</span><span>ARTIST</span>{{if eq .Source "music"}}<span>SOURCE</span><span>ADDED</span>{{end}}<span aria-label="Duration">TIME</span><span aria-label="Download">↓</span></div>
	        <ol class="tracks">{{range $i, $t := .Tracks}}<li data-source="{{$t.Source}}" data-artist="{{$t.Artist}}" data-title="{{$t.Title}}" data-playable="{{if $t.FilePath}}true{{else}}false{{end}}"><span class="track-number"><span class="number">{{add $i 1}}</span><button type="button" class="play-track" {{if not $t.FilePath}}disabled{{end}} aria-label="Play {{$t.Title}}">{{template "icon" "play"}}</button></span><strong>{{$t.Title}}</strong><span class="artist">{{if $t.Artist}}{{$t.Artist}}{{else}}Unknown artist{{end}}</span>{{if eq $.Source "music"}}<span class="source-tag {{if eq $t.Source "spotify"}}spotify{{else}}vk{{end}}">{{if eq $t.Source "spotify"}}Spotify{{else}}VK Music{{end}}</span><time class="added-date">{{addedDate $t.AddedAt}}</time>{{end}}<span class="track-duration">—</span><a class="download-track{{if not $t.FilePath}} unavailable{{end}}" href="/audio?source={{$t.Source | urlquery}}&amp;artist={{$t.Artist | urlquery}}&amp;title={{$t.Title | urlquery}}&amp;download=1" aria-label="Download {{$t.Title}} as MP3" title="Download MP3"{{if not $t.FilePath}} aria-disabled="true" tabindex="-1"{{end}}>{{template "icon" "download-01"}}</a></li>{{end}}</ol>
      {{else}}
        <div class="empty"><span>{{template "icon" "disc"}}</span><h3>Nothing here yet</h3><p>{{if eq .Source "music"}}Connect and sync Spotify or VK Music to fill your library.{{else if .Connected}}Run your first sync to see tracks from {{.Name}}.{{else}}Connect {{.Name}} above, then sync your library.{{end}}</p></div>
      {{end}}
	    </section>
	    {{end}}
	    {{if ne .Source "music"}}<footer>LOCAL DOWNLOADS · TRACKHARBOR</footer>{{end}}
  </main>
</div>
</body></html>{{define "icon"}}<svg class="icon" aria-hidden="true" focusable="false"><use href="/assets/icons.svg#hg-{{.}}"></use></svg>{{end}}`))
