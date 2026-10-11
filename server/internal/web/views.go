package web

import "html/template"

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TrackHarbor · {{.Name}}</title>
  <link rel="stylesheet" href="/assets/app.css">
  <script src="/assets/htmx.min.js" defer></script>
</head>
<body>
<div class="shell">
  <aside class="sidebar">
    <div class="brand"><span class="mark">◉</span><div><strong>TrackHarbor</strong><small>Your music, your harbor.</small></div></div>
    <div class="nav-label">YOUR LIBRARY</div>
    <nav aria-label="Sources">
      <a class="{{if eq .Source "spotify"}}active{{end}}" href="/?source=spotify" hx-get="/?source=spotify" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>◈</span> Spotify <span class="arrow">↗</span></a>
      <a class="{{if eq .Source "vk"}}active{{end}}" href="/?source=vk" hx-get="/?source=vk" hx-target="#content" hx-select="#content" hx-swap="outerHTML" hx-push-url="true"><span>♫</span> VK Music <span class="arrow">↗</span></a>
    </nav>
    <div class="sidebar-bottom"><span class="dot"></span> LOCAL SERVER <p>This preview runs on your computer only. No web login is configured.</p></div>
  </aside>
  <main id="content" data-source="{{.Source}}" hx-history-elt>
    <div class="topline"><span>LIBRARY / {{.Name}}</span><span class="label">EARLY ACCESS</span></div>
    <header class="hero"><div class="eyebrow">YOUR COLLECTION, IN ONE PLACE</div><h1>{{.Name}}</h1><p>Connect your account, sync your library, and explore what you love. Each source has its own collection.</p></header>
    <section class="grid">
      <div class="panel connect"><div class="panel-top"><span class="index">01 / CONNECTION</span><span class="pill {{if .Connected}}connected{{end}}">{{if .Connected}}● Connected{{else}}○ Not connected{{end}}</span></div>
        <h2>Connect {{.Name}}</h2>
        {{if eq .Source "spotify"}}
        <p>Paste the <code>authorization</code> and <code>client-token</code> headers from a Spotify Web Player request. Do not paste cookies.</p>
        <form action="/connect" method="post" autocomplete="off"><input type="hidden" name="source" value="spotify">
          <label>AUTHORIZATION<input name="authorization" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Bearer …" required></label>
          <label>CLIENT-TOKEN<input name="client_token" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Client token" required></label>
          <button type="submit">Save credentials <span>↗</span></button>
        </form>
        {{else}}
        <p><a href="{{.AuthURL}}" target="_blank" rel="noopener noreferrer">Open VK authorization ↗</a>, then paste the token or full redirect URL below. The redirect fragment stays in your browser until you paste it.</p>
        <form action="/connect" method="post" autocomplete="off"><input type="hidden" name="source" value="vk">
          <label>ACCESS TOKEN OR REDIRECT URL<input name="token" type="text" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" placeholder="Paste token or redirect URL" required></label>
          <button type="submit">Save credentials <span>↗</span></button>
        </form>
        {{end}}
      </div>
      <div class="panel sync"><div class="panel-top"><span class="index">02 / SYNC</span><span class="mini-mark">↻</span></div>
        <h2>Bring your tracks in</h2>
        <p>Fetch your latest liked tracks. Your current list stays available if a sync fails.</p>
        <form action="/sync" method="post"><input type="hidden" name="source" value="{{.Source}}"><button type="submit" class="secondary" {{if not .Connected}}disabled{{end}}>Sync library <span>→</span></button></form>
        <div class="status" aria-live="polite">STATUS <span id="sync-state" hx-get="/status?source={{.Source}}" hx-trigger="every 3s" hx-swap="innerHTML">{{if .Status}}{{.Status}}{{else}}Ready to sync.{{end}}</span></div>
      </div>
    </section>
    <section class="library"><div class="library-heading"><div><span class="index">03 / COLLECTION</span><h2>Your tracks</h2></div><span class="count">{{len .Tracks}} TRACKS</span></div>
      {{if .Tracks}}
        <div class="track-head"><span>#</span><span>TRACK</span><span>ARTIST</span></div>
        <ol class="tracks">{{range $i, $t := .Tracks}}<li><span class="number">{{add $i 1}}</span><strong>{{$t.Title}}</strong><span class="artist">{{if $t.Artist}}{{$t.Artist}}{{else}}Unknown artist{{end}}</span></li>{{end}}</ol>
      {{else}}
        <div class="empty"><span>◌</span><h3>Nothing here yet</h3><p>{{if .Connected}}Run your first sync to see tracks from {{.Name}}.{{else}}Connect {{.Name}} above, then sync your library.{{end}}</p></div>
      {{end}}
    </section>
    <footer>PLAYBACK AND DOWNLOADS ARE COMING LATER · TRACKHARBOR</footer>
  </main>
</div>
</body></html>`))
