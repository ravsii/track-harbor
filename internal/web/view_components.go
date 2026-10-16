package web

// Both server-rendered lists and the browser's download rows use these elements.
const componentTemplates = `
{{define "media-cell"}}
<div class="media-cell">
  <span class="track-cover" aria-hidden="true">{{template "brand-mark"}}<img{{with coverURL .CoverURL}} src="{{.}}"{{else}} hidden{{end}} alt="" loading="lazy"></span>
  <div class="media-text"><strong class="track-title">{{.Title}}</strong><span class="artist">{{if .Artist}}{{.Artist}}{{else}}Unknown artist{{end}}</span></div>
</div>
{{end}}

{{define "source-label"}}<span class="source-label">{{template "icon" sourceIcon .}}<span class="source-name">{{sourceTitle .}}</span></span>{{end}}

{{define "track-status"}}
<div class="row-status">
  <span class="status download-stage" data-stage="{{if .FilePath}}downloaded{{else}}waiting{{end}}">{{if .FilePath}}{{template "icon" "checkmark-circle-02"}}{{else}}{{template "icon" "circle"}}{{end}}<span class="status-label">{{if .FilePath}}Ready{{else}}Queued{{end}}</span></span>
  <progress max="100" value="0" aria-label="Download progress" hidden></progress>
</div>
{{end}}

{{define "track-list"}}
{{$personal := eq .Source "music"}}{{$catalog := eq .Source "all"}}
<div class="library media-list {{if $personal}}personal{{else if $catalog}}catalog{{end}}" aria-label="{{if eq .Source "youtube"}}Your videos{{else}}Tracks{{end}}">
  {{if .Tracks}}
  <div class="list-heading" aria-hidden="true"><span></span><span>Track</span>{{if or $personal $catalog}}<span>Source</span>{{end}}<span>{{if $personal}}Added{{else}}Status{{end}}</span><span>Time</span><span></span></div>
  <ol class="tracks">{{range .Tracks}}
    <li class="media-row" data-id="{{.ID}}" data-source="{{.Source}}" data-source-id="{{.SourceID}}" data-artist="{{.Artist}}" data-title="{{.Title}}" data-playable="{{if .FilePath}}true{{else}}false{{end}}">
      <button class="icon-button play-track" type="button"{{if not .FilePath}} disabled{{end}} aria-label="Play {{.Title}}" title="Play {{.Title}}">{{template "icon" "play"}}</button>
      {{template "media-cell" .}}
      {{if or $personal $catalog}}{{template "source-label" .Source}}{{end}}
      {{if $personal}}<time class="added-date" title="Added {{addedDate .AddedAt}}">{{addedDate .AddedAt}}</time>{{else}}{{template "track-status" .}}{{end}}
      <span class="track-duration" aria-label="Duration">—</span>
      <a class="icon-button download-track{{if not .FilePath}} unavailable{{end}}" href="/audio?id={{.ID}}&amp;download=1" aria-label="Download {{.Title}} as MP3" title="Download MP3"{{if not .FilePath}} aria-disabled="true" tabindex="-1"{{end}}>{{template "icon" "download-01"}}</a>
    </li>
  {{end}}</ol>
  {{else}}
  <div class="empty"><strong>{{if eq .Source "youtube"}}No videos added yet{{else if $catalog}}No tracks on this server yet{{else}}No tracks yet{{end}}</strong>
    {{if ne .Source "youtube"}}<p class="helper">{{if $personal}}Add a YouTube video or connect and sync a source.{{else if $catalog}}Tracks appear here when a user adds or syncs a source.{{else if .Connected}}Sync your library to bring in your tracks.{{else}}Connect {{.Name}} above, then sync your library.{{end}}</p>{{end}}
  </div>
  {{end}}
</div>
{{end}}

{{define "download-row-template"}}
<template id="download-row-template"><li class="media-row">{{template "media-cell" emptyTrack}}{{template "source-label" ""}}{{template "track-status" emptyTrack}}</li></template>
{{end}}
`
