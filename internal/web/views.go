package web

import (
	"html/template"
	"net/url"
	"time"
)

func coverURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return raw
}

const iconTemplate = `{{define "icon"}}<svg class="icon" aria-hidden="true" focusable="false"><use href="/assets/icons.svg#hg-{{.}}"></use></svg>{{end}}`

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"add":      func(a, b int) int { return a + b },
	"coverURL": coverURL,
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
  <meta name="htmx-config" content='{"includeIndicatorStyles":false,"allowEval":false,"allowScriptTags":false,"historyCacheSize":0}'>
  <link rel="stylesheet" href="/assets/tailwind.css">
	  <script src="/assets/theme.js"></script>
		  <script src="/assets/htmx.min.js" defer></script>
		  <script src="/assets/auth.js" defer></script>
	  <script src="/assets/download.js" defer></script>
	  <script src="/assets/settings.js" defer></script>
	  <script src="/assets/player.js" defer></script>
	  <script src="/assets/duration.js" defer></script>
</head>
<body data-user-id="{{.User.ID}}" hx-headers='{"X-TrackHarbor-User":"{{.User.ID}}"}'>
<div class="shell grid min-h-screen grid-cols-[256px_minmax(0,1fr)] max-[950.02px]:grid-cols-1">
  <header id="music-player" class="topbar sticky top-0 z-10 border-b border-edge bg-surface" aria-label="Music player">
    <div class="topbar-content">
      <div class="topbar-brand">
        <a class="brand" href="/" hx-get="/" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true" aria-label="TrackHarbor home"><span class="mark" aria-hidden="true"><i></i><i></i><i></i></span><strong>TrackHarbor</strong></a>
        <a id="system-status" href="/?source=downloads" hx-get="/?source=downloads" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true" aria-label="Checking status. Open Downloads" title="Checking status · Open Downloads"><span class="system-indicator" aria-hidden="true">{{template "icon" "circle"}}</span><span id="system-status-text">Checking</span></a>
      </div>
      <audio id="player-audio" preload="none"></audio>
      <div class="player-controls">
        <div class="player-transport">
          <button type="button" id="player-shuffle" aria-label="Shuffle" aria-pressed="false" title="Shuffle off">{{template "icon" "shuffle"}}</button>
          <button type="button" id="player-prev" aria-label="Previous track">{{template "icon" "previous"}}</button>
          <button type="button" id="player-toggle" class="primary" aria-label="Play">{{template "icon" "play"}}</button>
          <button type="button" id="player-next" aria-label="Next track">{{template "icon" "next"}}</button>
        </div>
        <div class="player-track-group">
          <span class="track-cover player-cover relative grid size-8 shrink-0 place-items-center overflow-hidden border border-edge bg-selected text-muted" aria-hidden="true">{{template "icon" "disc-album"}}<img id="player-cover-image" class="absolute inset-0 size-full object-cover" alt="" hidden></span>
          <div class="player-progress">
            <div class="player-track-line"><strong id="player-track" aria-live="polite"></strong></div>
            <div class="player-progress-row">
              <input type="range" id="player-seek" min="0" max="1000" value="0" aria-label="Seek">
              <span id="player-time">0:00 / 0:00</span>
            </div>
          </div>
        </div>
        <div class="player-volume-group">
          <button type="button" id="player-mute" aria-label="Mute" title="Mute">{{template "icon" "volume-high"}}</button>
          <input type="range" id="player-volume" min="0" max="100" value="80" aria-label="Volume">
        </div>
      </div>
      <div class="topbar-actions">
        <details id="account-menu" class="account-menu">
          <summary><span class="account-name">{{.User.Username}}</span>{{template "icon" "arrow-right-01"}}</summary>
          <div class="account-dropdown">
            <button type="button" id="theme-choice" class="account-menu-item" aria-label="Change color theme">{{template "icon" "sun-01"}}<span>Theme</span><span class="theme-value" data-theme-value></span></button>
            <hr>
            <form id="logout-form" method="post" action="/logout?account_id={{.User.ID}}"><button type="submit" class="account-menu-item">{{template "icon" "logout-01"}}<span>Sign out</span></button></form>
          </div>
        </details>
      </div>
      <div id="player-error" role="status" aria-live="polite"></div>
      <div id="bootstrap-warning" class="bootstrap-warning" role="status"{{if not .BootstrapWarning}} hidden{{end}}>Создайте отдельного пользователя, удалите admin</div>
    </div>
  </header>
  <aside class="sidebar border-r border-edge bg-sidebar">
	    <div class="nav-label">NAVIGATION</div>
	    <nav aria-label="Navigation">
	      <a class="{{if eq .Source "music"}}active{{end}}" href="/" hx-get="/" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>{{template "icon" "music-note-01"}}</span> Tracks <span class="arrow">{{template "icon" "arrow-right-01"}}</span></a>
	      <a class="{{if eq .Source "all"}}active{{end}}" href="/?source=all" hx-get="/?source=all" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>{{template "icon" "disc-album"}}</span> Server tracks <span class="arrow">{{template "icon" "arrow-right-01"}}</span></a>
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
    <section class="grid grid-cols-[1.15fr_.85fr] gap-[6px] max-[950.02px]:grid-cols-1">
        <div class="panel connect min-w-0 rounded-2xl border border-edge bg-surface p-[9px]"><div class="panel-top"><span class="index">01 / CONNECTION</span><span class="pill {{if .Connected}}connected{{end}}">{{if .Connected}}{{template "icon" "checkmark-circle-02"}} Connected{{else}}{{template "icon" "circle"}} Not connected{{end}}</span></div>
	        <h2>{{if .Connected}}Connection{{else}}Connect {{.Name}}{{end}}</h2>
	        {{if .Connected}}<details class="credential-details"><summary>Update credentials</summary>{{end}}
	        {{if eq .Source "spotify"}}
        <p>Paste the full request headers from a Spotify Web Player request. Only <code>authorization</code> and <code>client-token</code> are saved; other headers and cookies are discarded.</p>
        <form action="/connect?account_id={{$.User.ID}}" method="post" autocomplete="off"><input type="hidden" name="source" value="spotify">
          <label>REQUEST HEADERS<textarea name="headers" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="authorization: Bearer …&#10;client-token: …" required></textarea></label>
	          <button type="submit"><span>{{template "icon" "checkmark-circle-02"}}</span> Save credentials</button>
        </form>
        {{else}}
        <p><a href="{{.AuthURL}}" target="_blank" rel="noopener noreferrer">Open VK authorization {{template "icon" "external-link"}}</a>, then paste the token or full redirect URL below. The redirect fragment stays in your browser until you paste it.</p>
        <form action="/connect?account_id={{$.User.ID}}" method="post" autocomplete="off"><input type="hidden" name="source" value="vk">
          <label>ACCESS TOKEN OR REDIRECT URL<input name="token" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Paste token or redirect URL" required></label>
	          <button type="submit"><span>{{template "icon" "checkmark-circle-02"}}</span> Save credentials</button>
        </form>
	        {{end}}
	        {{if .Connected}}</details>{{end}}
		        <form action="/ping?account_id={{$.User.ID}}" method="post"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="secondary" {{if not .Connected}}disabled{{end}}><span>{{template "icon" "reload"}}</span> Check connection</button></form>
        <div class="status" aria-live="polite">CONNECTION <span hx-get="/ping-status?source={{.Source}}" hx-trigger="every 3s" hx-swap="innerHTML">{{if .PingStatus}}{{.PingStatus}}{{else}}Not checked yet.{{end}}</span></div>
      </div>
      <div class="panel sync min-w-0 rounded-2xl border border-edge bg-surface p-[9px]"><div class="panel-top"><span class="index">02 / SYNC</span><span class="mini-mark">{{template "icon" "reload"}}</span></div>
        <h2>Bring your tracks in</h2>
	        <p>Fetch your latest liked tracks, then check and download missing audio in the background. Your current list stays available if a sync fails.</p>
		        <form action="/sync?account_id={{$.User.ID}}" method="post"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="secondary" {{if not .Connected}}disabled{{end}}><span>{{template "icon" "reload"}}</span> Sync library</button></form>
        <div class="status" aria-live="polite">STATUS <span id="sync-state" hx-get="/status?source={{.Source}}" hx-trigger="every 10s" hx-swap="innerHTML">{{if .Status}}{{.Status}}{{else}}Ready to sync.{{end}}</span></div>
      </div>
    </section>
    {{end}}
	    {{if eq .Source "settings"}}
    <section class="panel settings-panel users-panel min-w-0 rounded-2xl border border-edge bg-surface p-[9px]">
      <h2>Users</h2><p>All users can manage accounts. Source credentials and playlists remain personal. Changing a password signs that user out everywhere.</p>
      <form id="create-user-form" action="/settings/users?account_id={{$.User.ID}}" method="post" hx-post="/settings/users" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-confirm="Create this user?">
        <label>Username<input name="username" autocomplete="off" pattern="[A-Za-z0-9._\-]{1,64}" maxlength="64" required></label>
        <label>Password<input type="password" name="password" autocomplete="new-password" required></label>
        <button type="submit">{{template "icon" "checkmark-circle-02"}} Create user</button>
      </form>
      <div id="users-feedback" class="account-error" role="status" aria-live="polite"></div>
      <ul class="user-list m-0 list-none p-0">{{range .Users}}
        <li class="user-row" data-username="{{.Username}}"><strong>{{.Username}}{{if eq .ID $.User.ID}} (you){{end}}</strong>
          <form class="user-password" action="/settings/users/{{.ID}}/password?account_id={{$.User.ID}}" method="post" hx-post="/settings/users/{{.ID}}/password" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-confirm="Change this user's password and sign out all their sessions?">
            <label>New password for {{.Username}}<input type="password" name="password" autocomplete="new-password" required></label><button type="submit" class="secondary">Change password</button>
          </form>
          <form class="user-delete" action="/settings/users/{{.ID}}/delete?account_id={{$.User.ID}}" method="post" hx-post="/settings/users/{{.ID}}/delete" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-confirm="Delete this user, their source credentials and playlist? Shared music files will be kept."><input type="hidden" name="confirm" value="yes"><button type="submit" class="secondary">{{template "icon" "delete-02"}} Delete user</button></form>
        </li>{{end}}</ul>
    </section>
    <section class="panel settings-panel min-w-0 rounded-2xl border border-edge bg-surface p-[9px]">
	      <p>Choose the connection routes for new download batches. Downloads already running keep their current routes.</p>
		      <form id="settings-form" action="/settings?account_id={{$.User.ID}}" method="post">
	        <label>SPOTIFY / YOUTUBE ROUTE ORDER<textarea name="strategy" required spellcheck="false">{{.SettingsStrategy}}</textarea></label>
	        <p>One per line, in priority order: <code>direct</code>, <code>personal_proxy</code>, <code>random_proxy</code>.</p>
	        <label>PUBLIC PROXIES<textarea name="public_proxy" spellcheck="false" autocomplete="off" placeholder="One proxy URL per line. Leave empty to keep saved values."></textarea></label><p><span id="public-proxy-count">{{.PublicProxyCount}}</span> saved. Addresses are never shown again; enter new URLs to replace the entire list.</p><label class="clear-option"><input type="checkbox" name="clear_public_proxy"> Remove saved public proxies</label>
	        <p>VK always uses <code>direct</code> (the only supported route). Set <code>listen</code> and <code>download_dir</code> in <code>config.yaml</code>; changing either requires a restart.</p>
	        <button type="submit"><span>{{template "icon" "checkmark-circle-02"}}</span> Save settings</button>
	      </form>
	      <div id="settings-feedback" class="status" role="status" aria-live="polite">{{if .SettingsSaved}}Settings saved. New download batches will use them.{{end}}</div>
		      <form id="public-proxy-import-form" action="/settings/public/import?account_id={{$.User.ID}}" method="post">
	        <button type="submit" class="secondary" {{if .ProxyImport.Running}}disabled{{end}}>{{template "icon" "reload"}}<span>Import random proxies</span></button>
	      </form>
	      <p>Import from ProxyScrape, Proxifly and TheSpeedX. Check every discovered host:port against YouTube; replace matching entries if working, remove them if not. Other saved proxies stay unchanged.</p>
	      <div id="public-proxy-import-status" class="status" role="status" aria-live="polite" data-running="{{.ProxyImport.Running}}">{{.ProxyImport.Message}}</div>
	    </section>
    <section class="panel settings-panel personal-proxies min-w-0 rounded-2xl border border-edge bg-surface p-[9px]">
	      <h2>Personal proxies</h2>
	      <p>Each new proxy must reach YouTube over HTTPS before it is saved. Downloads already running keep their current routes.</p>
		      <form id="personal-proxy-form" action="/settings/personal?account_id={{$.User.ID}}" method="post" autocomplete="off">
	        <label>PROXY URL<input name="address" type="text" required autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="http://login:password@host:port"></label>
	        <button type="submit"><span>{{template "icon" "checkmark-circle-02"}}</span> Check and add proxy</button>
	      </form>
	      <div id="personal-proxy-feedback" class="status" role="status" aria-live="polite"></div>
	      {{if .PersonalProxies}}
	      <ul class="personal-proxy-list">
	        {{range .PersonalProxies}}
	        <li data-proxy-id="{{.ID}}" data-proxy-host="{{.Host}}"><span class="proxy-address" data-masked="{{.Masked}}">{{.Masked}}</span>
	          <button type="button" class="secondary proxy-reveal" aria-expanded="false" aria-label="Show proxy URL for {{.Host}}">{{template "icon" "view"}}<span class="proxy-reveal-label">Show URL</span></button>
		          <form class="proxy-delete" method="post" action="/settings/personal/{{.ID}}/delete?account_id={{$.User.ID}}"><button type="submit" class="secondary" aria-label="Remove proxy {{.Host}}">{{template "icon" "delete-02"}}<span>Remove</span></button></form>
	        </li>
	        {{end}}
	      </ul>
	      {{else}}<p>No personal proxies saved yet.</p>{{end}}
	    </section>
	    {{else if eq .Source "downloads"}}
    <section class="panel download-panel min-w-0 rounded-2xl border border-edge bg-surface p-[9px]"><div class="panel-top"><span class="index">DOWNLOAD CONTROL</span></div>
	      <button type="button" id="download-toggle" disabled>{{template "icon" "play"}}<span class="download-toggle-label">Start downloads</span></button>
	      <div id="download-state" class="status" role="status" aria-live="polite">Loading download status…</div>
	    </section>
	    <section class="library download-library"><div class="library-heading"><h2>My tracks</h2></div><div class="track-head"><span>TRACK</span><span>ARTIST</span><span>SOURCE</span><span>STATUS</span></div><ol class="tracks" id="my-download-tracks"></ol></section>
	    <section class="library download-library"><div class="library-heading"><div><span class="index">SERVER</span><h2>Shared download queue</h2></div><span class="count" id="download-count"></span></div>
	      <div class="track-head"><span>TRACK</span><span>ARTIST</span><span>SOURCE</span><span>STATUS</span></div>
	      <ol class="tracks" id="download-tracks"></ol>
	    </section>
	    {{else}}
	    <section class="library {{if eq .Source "music"}}music-library{{end}}"{{if eq .Source "music"}} aria-label="Tracks"{{end}}>{{if ne .Source "music"}}<div class="library-heading"><div><span class="index">03 / COLLECTION</span><h2>Your tracks</h2></div><span class="count">{{len .Tracks}} TRACKS</span></div>{{end}}
      {{if .Tracks}}
        <div class="track-head grid items-center"><span>#</span><span>TRACK</span><span>ARTIST</span>{{if eq .Source "music"}}<span>SOURCE</span><span>ADDED</span>{{end}}<span aria-label="Duration">TIME</span></div>
        <ol class="tracks m-0 list-none border-t border-edge p-0">{{range $i, $t := .Tracks}}
          <li data-id="{{$t.ID}}" data-source="{{$t.Source}}" data-source-id="{{$t.SourceID}}" data-artist="{{$t.Artist}}" data-title="{{$t.Title}}" data-playable="{{if $t.FilePath}}true{{else}}false{{end}}">
            <span class="track-number"><span class="number">{{add $i 1}}</span><button type="button" class="play-track" {{if not $t.FilePath}}disabled{{end}} aria-label="Play {{$t.Title}}">{{template "icon" "play"}}</button><span class="playing-indicator" aria-hidden="true"><span></span><span></span><span></span></span></span>
            <strong class="flex min-w-0 items-center gap-2"><span class="track-cover relative grid size-7 shrink-0 place-items-center overflow-hidden rounded border border-edge bg-selected text-muted" aria-hidden="true">{{template "icon" "disc-album"}}{{with coverURL $t.CoverURL}}<img class="absolute inset-0 size-full object-cover" src="{{.}}" alt="" loading="lazy">{{end}}</span><span class="track-title">{{$t.Title}}</span></strong>
            <span class="artist">{{if $t.Artist}}{{$t.Artist}}{{else}}Unknown artist{{end}}</span>{{if eq $.Source "music"}}<span class="source-tag {{if eq $t.Source "spotify"}}spotify{{else}}vk{{end}}">{{if eq $t.Source "spotify"}}Spotify{{else}}VK Music{{end}}</span><time class="added-date">{{addedDate $t.AddedAt}}</time>{{end}}
            <span class="track-duration">—</span><a class="download-track{{if not $t.FilePath}} unavailable{{end}}" href="/audio?id={{$t.ID}}&amp;download=1" aria-label="Download {{$t.Title}} as MP3" title="Download MP3"{{if not $t.FilePath}} aria-disabled="true" tabindex="-1"{{end}}>{{template "icon" "download-01"}}</a>
          </li>{{end}}</ol>
      {{else}}
        <div class="empty"><span>{{template "icon" "disc"}}</span><h3>Nothing here yet</h3><p>{{if eq .Source "music"}}Connect and sync Spotify or VK Music to fill your library.{{else if .Connected}}Run your first sync to see tracks from {{.Name}}.{{else}}Connect {{.Name}} above, then sync your library.{{end}}</p></div>
      {{end}}
	    </section>
	    {{end}}
	    {{if ne .Source "music"}}<footer>LOCAL DOWNLOADS · TRACKHARBOR</footer>{{end}}
  </main>
</div>
</body></html>` + iconTemplate))
