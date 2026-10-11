# TrackHarbor

TrackHarbor is a local web application that syncs Spotify Liked Songs and VK My
Music, downloads their audio as MP3 files, and plays saved tracks in the browser.
Spotify tracks are matched to YouTube audio; VK audio is downloaded from VK.

## Run locally

Install Go, FFmpeg, and `yt-dlp` (for Spotify audio), then run:

```sh
make run
```

Open <http://127.0.0.1:8787>. Connect a source on its tab and click **Sync
library** to import tracks and check or download missing MP3s in the background.
The **Downloads** tab shows tracks still waiting, downloading, or failed; saved
MP3s leave the queue. The top bar shows overall status
and keeps the player running as you navigate. Use `make run-server-watch` to
rebuild and restart automatically when server source changes; failed builds
are retried every second without stopping the previous server.
The global **Stop downloads** button on Downloads cancels ongoing transfers and
blocks automatic downloads after further syncs. **Start downloads** checks the
entire current library and resumes downloading missing tracks.
On first startup, downloads are enabled and the saved library is checked in
the background for missing MP3s. Manual Stop/Start choices persist across
server restarts.

The server writes `config.yaml`, `library.db`, and `downloads/` beside its
executable. `make run` builds the executable in the Git-ignored `.local-server/`
directory so local development data stays there. Use `TRACKHARBOR_SERVER_DIR`
to change the default directory, or `TRACKHARBOR_SERVER_CONFIG` and
`TRACKHARBOR_SERVER_DB` to set those paths independently. Existing server data
is not moved automatically; keep using the same data directory when upgrading.
Server database migrations in `internal/store/migrations/` run on startup.

Server logs are structured JSON on stderr at Info level. Downloads use
`downloads/youtube/<video-id>.mp3` for Spotify library tracks and
`downloads/vk/<owner-id>_<audio-id>.mp3` for VK. Existing MP3 paths recorded in
the server database remain playable. SSE notifies the browser only when a new
MP3 is saved; the Downloads tab polls for status. MP3 seeking uses HTTP range requests.
The player remembers the last played track in this browser. On reload it shows
that track paused if its MP3 is available, or the most recently added
available track otherwise; playback never starts automatically.

The web UI has **no login** and binds to loopback by default. Do not expose it
to a network without access control. The old CLI is archived in
`.deprecated/cli/`; it is no longer supported or part of the build, and its
`~/.trackharbor` database is never used by the server.

## Development

```sh
make build            # build the server executable in .local-server/
go build ./...         # compile the root Go module
make run-server-watch  # watch server source and restart after successful builds
```

The interface uses local HTMX and Hugeicons assets under `internal/web/assets/`.
See `docs/visual-design.md` for theme and layout conventions and
`docs/hugeicons-license.txt` for icon attribution.
