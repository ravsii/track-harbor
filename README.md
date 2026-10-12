# TrackHarbor

TrackHarbor is a local web application that syncs Spotify Liked Songs and VK My
Music, downloads their audio as MP3 files, and plays saved tracks in the browser.
Spotify tracks are matched to YouTube audio; VK audio is downloaded from VK.

## Run locally

Install Go, FFmpeg, and `yt-dlp` (for Spotify audio), then run:

```sh
make run
```

Open <http://127.0.0.1:8787> in a browser that allows Secure cookies on loopback
(use HTTPS otherwise). Sign in with **admin / admin**, create your own user in
**Settings**, sign in as that user and delete `admin`. There is no public
registration; every authenticated user can create users, reset any user's password
or delete accounts. The last user cannot be deleted. A warning remains visible
until `admin` is deleted; the default password is not safe for public deployment.

Connect your sources on their tabs and click **Sync
library** to import tracks and check or download missing MP3s in the background.
Each account has one personal track list populated from its Spotify Liked Songs
and VK My Music. Tokens and sync status belong to that account. Tracks removed
from a source disappear from that account's list on the next sync, not from
other users' lists or shared files. Track identity is `(source, source_id)`, not
artist/title; the same track from different sources remains separate.
The **Server tracks** tab gives any authenticated user access to the shared
catalogue and saved audio. The **Downloads** tab separates your tracks from the
shared queue of tracks still waiting, downloading, or failed; saved
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

Each tab has its own player: playback, seeking, shuffle and volume do not affect
other tabs or browsers. In-page navigation preserves playback; opening/reloading
a tab never starts playback automatically.

Sessions use signed JWTs in HttpOnly, Secure, SameSite=Lax cookies for seven days.
Signing keys and revocable session IDs persist in the server database. Signing
out revokes that session; changing a password revokes all that user's sessions,
including the current one. Deleting an account also removes its source credentials
and personal playlist but keeps the shared catalogue and MP3s. Passwords are
bcrypt hashes (1–72 UTF-8 bytes); usernames accept 1–64 ASCII letters, digits,
dots, underscores and hyphens and are case-sensitive.

The server writes `config.yaml`, `library.db`, and `downloads/` beside its
executable. `make run` builds the executable in the Git-ignored `.local-server/`
directory so local development data stays there. Use `TRACKHARBOR_SERVER_DIR`
to change the default directory, or `TRACKHARBOR_SERVER_CONFIG` and
`TRACKHARBOR_SERVER_DB` to set those paths independently. Existing server data
is not moved automatically; keep using the same data directory when upgrading.
Server database migrations in `internal/store/migrations/` run on startup.
Back up `library.db` before upgrading. The multi-user migration clears legacy
source credentials: reconnect Spotify and VK separately for each account.
Existing VK files with known owner/audio IDs enter the shared catalogue. Legacy
Spotify rows had no Spotify track IDs: their old file index and MP3s are kept,
but not guessed into a new playlist or shared catalogue by artist/title. A new
Spotify sync creates source-ID entries and may download their audio again.
Deleting `admin` is permanent across restarts; it is not automatically recreated.
The server embeds the checked-in `internal/web/assets/tailwind.css`, so
`go build ./...` and `make run` do not need Node.js. To change web styles,
install dependencies with `npm ci` and run `make css-build` before building
the Go server. `make css-watch` recompiles styles while editing;
`make run-server-watch` rebuilds Tailwind automatically after CSS, template or UI JS
edits when dependencies are installed, then restarts the server. Tailwind's
source is `internal/web/tailwind.css`; existing component/state styles in
`internal/web/assets/app.css` and `theme.css` remain bundled during the
incremental migration. Preflight is intentionally disabled to preserve the
current appearance. Commit the generated CSS with each style change.
Tailwind v4 targets Chrome 111+, Safari 16.4+ and Firefox 128+; browsers
older than these are not supported by the new utility styles.
Configure download route order and personal/public proxy lists on the local
**Settings** page; proxies, route order and download controls are shared by all
accounts. Changes affect new download batches; running transfers keep
their routes. Add personal proxies one at a time: each must reach YouTube over
HTTPS through that proxy before it is saved. The list masks credentials as
`socks5://********:********@host:port`; Show URL reveals them and provides the complete
proxy URL for copying. Removing an entry requires confirmation. Public proxy
URLs are not shown again: a blank list leaves its current values unchanged,
and the removal checkbox clears them. The Settings import button fetches public
proxies from ProxyScrape, Proxifly and TheSpeedX and checks every discovered
host:port for HTTPS access to YouTube in the background (up to 500 checks
concurrently, with a one-second timeout per proxy). It adds working addresses;
when an imported
host:port matches a saved public proxy, the saved entry is replaced if working
or removed if none of that host's protocols work. Other entries are kept;
there is no fixed limit on the number of imported working addresses. On upgrade,
the server imports existing `strategy`, `personal_proxy`, and `public_proxy`
values from `config.yaml` into `library.db` and removes those keys from YAML.
`vk_strategy` is fixed to `direct`. Keep `listen` and optional `download_dir`
in `config.yaml`; changes to either require a server restart. The server binds
to loopback by default and serves HTTP internally. For other devices or public
access, put it behind an **HTTPS reverse proxy**, keeping the HTTP port private
(loopback or a firewall). The proxy must preserve the external `Host` header,
support HTTP ranges and disable buffering for `/download/events`. POST requests
require an HTTP/HTTPS Origin matching Host; arbitrary forwarded headers are not
trusted for authentication. Direct HTTP over a LAN cannot use the Secure session
cookie. Restrict login attempts at the proxy as well: the built-in rate limit
uses the direct peer IP, so clients behind one proxy share its limit.
Protect the server database and backups: they contain source tokens, proxy
credentials, session IDs and the JWT signing key. Only trusted people should have
accounts: every user can change another user's password and access their account.

Server logs are structured JSON on stderr at Info level. Downloads use
`downloads/youtube/<hex-encoded-spotify-id>.mp3` for Spotify library tracks (the
encoding preserves case-sensitive IDs on case-insensitive filesystems) and
`downloads/vk/<owner-id>_<audio-id>.mp3` for VK. Existing MP3 paths recorded in
the shared catalogue remain playable. SSE notifies the browser when a new
MP3 is saved; polling also restores availability if an SSE event was missed.
MP3 seeking uses HTTP range requests.
The player remembers the last played track for this account in this tab. On reload it shows
that track paused if its MP3 is available, or the most recently added
available track otherwise; playback never starts automatically.

The old CLI is archived in
`.deprecated/cli/`; it is no longer supported or part of the build, and its
`~/.trackharbor` database is never used by the server.

## Development

```sh
make build            # build the server executable in .local-server/
go build ./...         # compile the root Go module
make run-server-watch  # watch server source and restart after successful builds
go test ./...          # backend tests (FFmpeg required), with isolated/local fixtures
go test -race ./...    # backend tests with race detection
npm ci
npm run assets:build   # regenerate imported Hugeicons sprite and compiled CSS
npx playwright install chromium --only-shell
npm run test:web       # real Chromium integration tests (also requires FFmpeg)
```

The interface uses local HTMX and Hugeicons assets under `internal/web/assets/`.
Icons are named imports from the pinned `@hugeicons/core-free-icons` dependency
in `scripts/build-icons.mjs`. Run `npm run icons:build` after adding an icon;
never edit the generated `icons.svg` by hand. Generated UI assets are committed
and embedded in the Go server, so Node.js is needed only for asset rebuilding,
not for building or running the server from the checked-in assets.
See `.agents/skills/design/SKILL.md` for shared UI element, theme, and spacing conventions and
`docs/hugeicons-license.txt` for icon attribution.
