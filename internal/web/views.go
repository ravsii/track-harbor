package web

import (
	"html/template"
	"net/url"
	"time"

	"github.com/ravsii/trackharbor/internal/store"
)

const (
	iconTemplate = `{{define "icon"}}
<svg class="icon" aria-hidden="true" focusable="false"><use href="/assets/icons.svg#hg-{{.}}"></use></svg>
{{end}}`
	brandMarkTemplate = `{{define "brand-mark"}}<span class="mark" aria-hidden="true"><i></i><i></i><i></i></span>{{end}}`
)

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"coverURL":    coverURL,
	"sourceTitle": sourceTitle,
	"sourceIcon":  sourceIcon,
	"emptyTrack":  func() store.Track { return store.Track{} },
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
  <script src="/assets/ui.js" defer></script>
  <script src="/assets/auth.js" defer></script>
  <script src="/assets/download.js" defer></script>
  <script src="/assets/settings.js" defer></script>
  <script src="/assets/youtube.js" defer></script>
  <script src="/assets/player.js" defer></script>
  <script src="/assets/duration.js" defer></script>
</head>
<body data-user-id="{{.User.ID}}" hx-headers='{"X-TrackHarbor-User":"{{.User.ID}}"}'>
<header id="app-header" class="fixed inset-x-0 top-0 z-10">
  <div class="mx-auto grid w-[calc(100%_-_48px)] max-w-7xl grid-cols-[232px_minmax(0,1fr)] gap-6 rounded-b-[10px] border-x border-b border-edge bg-surface max-[700px]:w-[calc(100%_-_40px)] max-[700px]:grid-cols-1 max-[700px]:gap-0">
    <div class="topbar-brand col-start-1 row-start-1 px-6 py-2 max-[700px]:px-5"><a class="brand" href="/" hx-get="/" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true" aria-label="TrackHarbor home">{{template "brand-mark"}}<strong>TrackHarbor</strong></a></div>
    <div id="music-player" class="topbar col-start-2 row-start-1 self-start max-[700px]:col-start-1 max-[700px]:row-start-2" role="region" aria-label="Music player">
      <div class="topbar-content @container grid w-full grid-cols-1 items-center gap-3 px-6 py-2 max-[700px]:px-5">
        <audio id="player-audio" preload="none"></audio>
        <div class="player-controls grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 @min-[560px]:grid-cols-[auto_minmax(0,1fr)_auto] @max-[300px]:grid-cols-1">
          <div class="player-transport col-start-1 row-start-1">
            <button type="button" class="icon-button" id="player-shuffle" aria-label="Shuffle" aria-pressed="false" title="Shuffle off">{{template "icon" "shuffle"}}</button>
            <button type="button" class="icon-button" id="player-prev" aria-label="Previous track" title="Previous track">{{template "icon" "previous"}}</button>
            <button type="button" class="icon-button" id="player-toggle" aria-label="Play" title="Play or pause">{{template "icon" "play"}}</button>
            <button type="button" class="icon-button" id="player-next" aria-label="Next track" title="Next track">{{template "icon" "next"}}</button>
          </div>
          <div class="player-track-group col-span-full row-start-2 @min-[560px]:col-span-1 @min-[560px]:col-start-2 @min-[560px]:row-start-1 @max-[300px]:row-start-3">
            <span class="track-cover" aria-hidden="true">{{template "brand-mark"}}<img id="player-cover-image" alt="" hidden></span>
            <div class="player-progress"><div class="player-track-line"><strong id="player-track" aria-live="polite"></strong></div><div class="player-progress-row"><input type="range" id="player-seek" min="0" max="1000" value="0" aria-label="Seek"><span id="player-time">0:00 / 0:00</span></div></div>
          </div>
          <div class="player-volume-group col-start-2 row-start-1 @min-[560px]:col-start-3 @max-[300px]:col-start-1 @max-[300px]:row-start-2 @max-[300px]:justify-self-end"><button type="button" class="icon-button" id="player-mute" aria-label="Mute" title="Mute">{{template "icon" "volume-high"}}</button><input type="range" id="player-volume" min="0" max="100" value="80" aria-label="Volume"></div>
        </div>
        <div id="player-error" class="feedback" data-error="true" role="status" aria-live="polite"></div>
        <div id="bootstrap-warning" class="bootstrap-warning" role="status"{{if not .BootstrapWarning}} hidden{{end}}>Создайте отдельного пользователя, удалите admin</div>
      </div>
    </div>
  </div>
</header>
<div class="shell mx-auto grid min-h-dvh w-[calc(100%_-_48px)] max-w-7xl grid-cols-[232px_minmax(0,1fr)] grid-rows-[minmax(0,1fr)] gap-6 pb-6 pt-[calc(var(--topbar-height,_64px)_+_24px)] max-[700px]:w-[calc(100%_-_40px)] max-[700px]:grid-cols-1 max-[700px]:grid-rows-[auto_minmax(0,1fr)] max-[700px]:pb-[22px]">
  <aside class="sidebar col-start-1 row-start-1" aria-label="Library sidebar">
    <details class="nav-disclosure" id="sidebar-navigation" open><summary><span>Library · <span id="sidebar-current-page">{{.Name}}</span></span>{{template "icon" "arrow-right-01"}}</summary><div class="sidebar-body">
      <nav class="grid grid-cols-1 gap-1.5" aria-label="Library">
        <a class="nav-link {{if eq .Source "music"}}active{{end}}" href="/" hx-get="/" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true">{{template "icon" "music-note-01"}}<span>Tracks</span><span class="sidebar-count" id="sidebar-personal-count">{{.Library.Personal}}</span></a>
        <a class="nav-link {{if eq .Source "all"}}active{{end}}" href="/?source=all" hx-get="/?source=all" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true">{{template "icon" "disc-album"}}<span>Server tracks</span><span class="sidebar-count" data-catalog-count>{{.Library.Total}}</span></a>
        <a class="nav-link {{if eq .Source "downloads"}}active{{end}}" href="/?source=downloads" hx-get="/?source=downloads" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true">{{template "icon" "download-01"}}<span>Downloads</span></a>
      </nav>
      <section class="border-t border-edge pt-[18px]">
        <h2 class="sidebar-label">Your sources</h2>
        <nav class="grid grid-cols-1 gap-1.5" aria-label="Your connected sources">
          <a class="nav-link sidebar-source {{if eq .Source "spotify"}}active{{end}}" data-source="spotify" href="/?source=spotify" hx-get="/?source=spotify" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"{{if not .Library.SpotifyConnected}} hidden{{end}}>{{template "icon" "spotify"}}<span>Spotify</span><span class="sidebar-count" data-source-count="spotify">{{.Library.Spotify}}</span></a>
          <a class="nav-link sidebar-source {{if eq .Source "vk"}}active{{end}}" data-source="vk" href="/?source=vk" hx-get="/?source=vk" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"{{if not .Library.VKConnected}} hidden{{end}}>{{template "icon" "vk"}}<span>VK Music</span><span class="sidebar-count" data-source-count="vk">{{.Library.VK}}</span></a>
          <a class="nav-link sidebar-source {{if eq .Source "youtube"}}active{{end}}" data-source="youtube" href="/?source=youtube" hx-get="/?source=youtube" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true">{{template "icon" "youtube"}}<span>YouTube</span><span class="sidebar-count" data-source-count="youtube">{{.Library.YouTube}}</span></a>
        </nav>
        <p class="sidebar-note" id="sidebar-no-sources"{{if or .Library.SpotifyConnected .Library.VKConnected}} hidden{{end}}>Spotify and VK are not connected.</p>
        <details class="source-connect" id="sidebar-connect"{{if and .Library.SpotifyConnected .Library.VKConnected}} hidden{{end}}><summary>Connect source</summary><nav class="grid grid-cols-1 gap-1.5" aria-label="Connect a source">
          <a class="nav-link" data-connect-source="spotify" href="/?source=spotify" hx-get="/?source=spotify" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"{{if .Library.SpotifyConnected}} hidden{{end}}>{{template "icon" "spotify"}}<span>Connect Spotify</span></a>
          <a class="nav-link" data-connect-source="vk" href="/?source=vk" hx-get="/?source=vk" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"{{if .Library.VKConnected}} hidden{{end}}>{{template "icon" "vk"}}<span>Connect VK Music</span></a>
        </nav></details>
      </section>
      <section class="sidebar-summary" aria-label="Server status"><h2 class="sidebar-label">Server status</h2><div class="sidebar-summary-row"><span>Total tracks</span><strong data-catalog-count>{{.Library.Total}}</strong></div><div class="sidebar-summary-row"><span>Sync</span><a id="sidebar-sync-status" href="/?source=downloads" hx-get="/?source=downloads" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true" title="Library synchronization and downloads across all users">{{template "icon" "circle"}}<span>{{.LibrarySync}}</span></a></div></section>
      <div class="sidebar-bottom grid grid-cols-1 gap-3">
        <nav aria-label="Settings"><a class="nav-link {{if eq .Source "settings"}}active{{end}}" href="/?source=settings" hx-get="/?source=settings" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true">{{template "icon" "computer"}}<span>Settings</span></a></nav>
        <div class="sidebar-account"><strong title="{{.User.Username}}">{{.User.Username}}</strong><div class="flex shrink-0 items-center gap-1"><button type="button" class="icon-button" id="theme-choice" title="Change color theme" aria-label="Change color theme">{{template "icon" "sun-01"}}</button><form id="logout-form" method="post" action="/logout?account_id={{.User.ID}}"><button type="submit" class="icon-button" title="Sign out" aria-label="Sign out {{.User.Username}}">{{template "icon" "logout-01"}}</button></form></div></div>
      </div>
    </div></details>
  </aside>
  <main id="content" class="surface content col-start-2 row-start-1 max-[700px]:col-start-1 max-[700px]:row-start-2" data-source="{{.Source}}" hx-history-elt>
    <header class="page-heading"><div class="page-identity"><h1>{{.Name}}</h1>{{if and (ne .Source "downloads") (ne .Source "settings")}}<span class="page-count">{{len .Tracks}} {{if eq .Source "youtube"}}videos{{else}}tracks{{end}}</span>{{end}}</div></header>
    {{if eq .Source "settings"}}{{template "settings" .}}
    {{else if eq .Source "downloads"}}{{template "downloads" .}}
    {{else if or (eq .Source "spotify") (eq .Source "vk")}}{{template "source" .}}
    {{else if eq .Source "youtube"}}
    <section class="content-section"><header class="section-heading"><h2>Add a video</h2></header><form class="form" id="youtube-add-form" action="/youtube/add?account_id={{.User.ID}}" method="post">
      <div class="grid grid-cols-[minmax(0,1fr)_auto] items-end gap-3 max-[700px]:grid-cols-1"><label class="field">YouTube link or video ID<input name="video" type="text" required autocomplete="off" spellcheck="false" placeholder="https://www.youtube.com/watch?v=…" aria-describedby="youtube-feedback"></label><div><button type="submit" class="button primary">{{template "icon" "youtube"}}<span>Add video</span></button></div></div><div id="youtube-feedback" class="feedback" role="status" aria-live="polite"></div>
    </form></section>
    <section class="content-section"><header class="section-heading"><h2>Your videos</h2></header>{{template "track-list" .}}</section>
    {{else}}{{template "track-list" .}}{{end}}
  </main>
</div>
</body></html>

{{define "source"}}
<section class="content-section">
  <header class="section-heading"><h2>Connection</h2><span class="status"{{if .Connected}} data-state="ready"{{end}}>{{if .Connected}}{{template "icon" "checkmark-circle-02"}}Connected{{else}}{{template "icon" "circle"}}Not connected{{end}}</span></header>
  <div class="actions">
    <form action="/sync?account_id={{.User.ID}}" method="post" hx-post="/sync" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-disabled-elt="find button" data-feedback="source-feedback"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="button primary"{{if or (not .Connected) (eq .Status "Syncing…")}} disabled{{end}}>{{template "icon" "reload"}}{{if eq .Status "Syncing…"}}Syncing…{{else}}Sync library{{end}}</button></form>
    <form action="/ping?account_id={{.User.ID}}" method="post" hx-post="/ping" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-disabled-elt="find button" data-feedback="source-feedback" data-source-ping><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="button"{{if not .Connected}} disabled{{end}}>{{template "icon" "reload"}}Check connection</button></form>
  </div>
  <div class="feedback" id="sync-state" aria-live="polite" hx-get="/status?source={{.Source}}" hx-trigger="every 10s" hx-swap="innerHTML">{{if .Status}}{{.Status}}{{else if .Connected}}Ready to sync.{{else}}Save your credentials to connect this source.{{end}}</div>
  <div class="feedback" id="connection-state" aria-live="polite" hx-get="/ping-status?source={{.Source}}" hx-trigger="every 3s" hx-swap="innerHTML">{{if .PingStatus}}{{.PingStatus}}{{end}}</div>
  <div class="feedback" id="source-feedback" role="status" aria-live="polite"></div>
  {{if .Connected}}<details class="disclosure"><summary>{{template "icon" "arrow-right-01"}}Update credentials</summary>{{end}}
  <form class="form" action="/connect?account_id={{.User.ID}}" method="post" hx-post="/connect" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-disabled-elt="find button" data-feedback="source-feedback" autocomplete="off">
    <input type="hidden" name="source" value="{{.Source}}">
    {{if eq .Source "spotify"}}
    <p class="helper">Paste the request headers from Spotify Web Player. Only authorization and client-token are saved; other headers and cookies are discarded.</p>
    <label class="field">Request headers<textarea name="headers" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="authorization: Bearer …&#10;client-token: …" aria-describedby="source-feedback" required></textarea></label>
    {{else}}
    <p class="helper"><a href="{{.AuthURL}}" target="_blank" rel="noopener noreferrer">Open VK authorization {{template "icon" "external-link"}}</a>, then paste the token or redirect URL.</p>
    <label class="field">Access token or redirect URL<input name="token" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Paste token or redirect URL" aria-describedby="source-feedback" required></label>
    {{end}}
    <div class="actions"><button type="submit" class="button{{if not .Connected}} primary{{end}}">{{template "icon" "checkmark-circle-02"}}Save credentials</button></div>
  </form>
  {{if .Connected}}</details>{{end}}
</section>
<section class="content-section"><header class="section-heading"><h2>Your tracks</h2></header>{{template "track-list" .}}</section>
{{end}}

{{define "downloads"}}
<section class="content-section"><header class="section-heading"><h2>Download control</h2></header><div class="actions"><button type="button" id="download-toggle" class="button" disabled>{{template "icon" "play"}}<span class="download-toggle-label">Start downloads</span></button><span id="download-summary-status" class="status">{{template "icon" "circle"}}<span>Loading…</span></span></div><div id="download-state" class="feedback" role="status" aria-live="polite">Loading download status…</div></section>
<section class="content-section"><header class="section-heading"><h2>My downloads</h2><span class="meta" id="my-download-count"></span></header><div class="library download-library media-list queue"><div class="list-heading" aria-hidden="true"><span>Track</span><span>Source</span><span>Status</span></div><ol class="tracks" id="my-download-tracks"></ol></div></section>
<section class="content-section"><header class="section-heading"><h2>Shared download queue</h2><span class="meta" id="download-count"></span></header><div class="library download-library media-list queue"><div class="list-heading" aria-hidden="true"><span>Track</span><span>Source</span><span>Status</span></div><ol class="tracks" id="download-tracks"></ol></div></section>
{{template "download-row-template"}}
{{end}}
` + iconTemplate + brandMarkTemplate + componentTemplates + settingsTemplate))

func coverURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return raw
}

func sourceTitle(source string) string {
	switch source {
	case "spotify":
		return "Spotify"
	case "vk":
		return "VK Music"
	case "youtube":
		return "YouTube"
	default:
		return source
	}
}

func sourceIcon(source string) string {
	switch source {
	case "spotify", "vk", "youtube":
		return source
	default:
		return "music-note-01"
	}
}
