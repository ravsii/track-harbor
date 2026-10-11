# Meshvault

Sync Spotify tracks to SQLite and download music to `~/.meshvault/downloaded`.

```sh
go run ./cmd/cli spotify auth
go run ./cmd/cli spotify sync
go run ./cmd/cli download
```

Downloads always use YouTube; there is no provider selection.
Running the CLI without a subcommand displays help and does not start downloads.

## Spotify

Spotify commands are available only under the `spotify` group:

- `spotify auth` prompts for the `authorization` and `client-token` headers
  from Spotify Web Player and replaces the saved tokens in the database.
  Finish input with a blank line. Cookies and other headers are not stored.
  Access is checked during the next `spotify sync`.
- `spotify sync` uses saved tokens, fetches library pages, and identifies
  Liked Songs by `format: liked-songs` or the `spotify:collection:tracks` URI,
  regardless of the display language. It then fetches all collection pages
  and atomically replaces the track list. If tokens are missing or expired,
  run `spotify auth` separately.

The old top-level `auth` and `sync` commands have been removed. Existing tokens,
tracks, and download history in `~/.meshvault/vault.db` are reused; no additional
database migration is required. No user-specific playlist ID is hardcoded.

`spotify sync` diagnostics are written to `logs/spotify-sync.jsonl`, relative to
the working directory. Each run has a `run_id`. The log includes API operations,
URIs, pagination parameters, HTTP statuses, Liked Songs identification details,
and sanitized API error messages. Tokens, cookies, and full requests or responses
are not logged. The file is created with `0600` permissions and appended to on
subsequent runs.

The application uses the web player's internal API, which may change. The
library is requested with the `LIKED_SONGS` feature flag. The discovered
`spotify:collection:tracks` URI is mapped to the built-in Liked Songs URI,
`spotify:playlist:37i9dQZF1F5p3rmiWPIYgZ`, defined in Spotify's public web-player
JavaScript. This is not a user-specific ID extracted from the library. The
response must have `format: liked-songs`; the collection URI is never passed to
`fetchPlaylist`. If synchronization fails, the previous track list is preserved.

## Database and configuration

The database is stored at `~/.meshvault/vault.db`, and settings are stored at
`~/.meshvault/config.yaml`. The home directory is resolved through the operating
system, independently of the current working directory. The directory and
configuration are created automatically on the first operational run:

```yaml
version: 1

download_dir: ~/.meshvault/downloaded
personal_proxy: []
strategy:
  - direct
```

If the new database does not exist, the application copies the contents of
`meshtune.db` from the working directory, including download history, Spotify
headers, and the proxy cache. The original database is retained as a backup;
an existing `vault.db` is never replaced. To migrate, run the application for
the first time from the directory containing the old database.
On systems with POSIX permissions, the application directory is created with
`0700` permissions, and the database and configuration use `0600`.

When configuration is loaded, missing fields are automatically added to
`config.yaml` with their current defaults: `version`, `strategy`,
`personal_proxy`, and `download_dir`. Existing values and YAML comments are
preserved. The `version` key always comes first, followed by one blank line;
other top-level keys are sorted alphabetically. The order of items in
`strategy` and `personal_proxy` is unchanged. If all fields and formatting are
already up to date, the file is not rewritten. Explicitly invalid values cause
an error rather than being silently corrected. Updates are saved through a
temporary file with `0600` permissions.

Override the download directory with `download_dir`:

```yaml
download_dir: ~/Music/MeshVault
```

If this setting is missing or empty, `~/.meshvault/downloaded` is used.
Absolute paths and `~/…` are supported. Relative paths are resolved against
`~/.meshvault`, not the working directory. Before processing tracks, the output
directory and its parents are created automatically. The path must be a writable
directory; otherwise, the command exits with a clear error before making network
requests. Old files in `tmp_music` are not moved automatically. To keep using
them, set `download_dir` to the absolute path of the old directory.

## Connection strategies

`strategy` is a non-empty priority list applied separately to each search and
download. For example, in `~/.meshvault/config.yaml`:

```yaml
version: 1

download_dir: ~/.meshvault/downloaded
personal_proxy:
  - "http://user:password@proxy.example.com:8080"
  - "socks5://127.0.0.1:1080"
strategy:
  - direct
  - personal_proxy
  - random_proxy
```

- `direct`: explicitly use a direct connection. If it is the only strategy,
  proxy lists are neither fetched nor checked.
- `personal_proxy`: use addresses from `personal_proxy`, trying them in order
  until one succeeds. If all personal proxies fail, the next strategy is used.
  At download startup, HTTPS access to YouTube's `robots.txt` is checked through
  each personal proxy with a five-second timeout and TLS verification. This is
  not an ICMP ping: it checks the actual proxy route, including SOCKS5/HTTP
  authentication. Unavailable addresses are excluded for the current run without
  changing the YAML list, and checked again on the next run. Each address's
  status is displayed without its username or password. If all are unavailable,
  the next strategy is used; if no routes are available at all, the command exits
  with a clear error. Domain names and local addresses are allowed. HTTP, HTTPS,
  SOCKS4, and SOCKS5 are supported, including URLs with credentials. Bare
  `host:port` entries use HTTP. The list must be non-empty when this strategy is
  enabled. Passwords are not shown in the UI or connection log.
- `random_proxy`: use verified public proxies from ProxyScrape, Proxifly, and
  TheSpeedX, without silently falling back to a direct connection. Despite its
  name, selection uses LRU with HTTP 429 cooldowns, not random selection.
  The old name `proxy` is accepted as an alias.
- In the example, a request first uses a direct connection. On a technical
  error, personal proxies are tried, then the public pool. Strategies can be
  ordered arbitrarily. An empty search result, cancellation, or an exhausted
  overall timeout does not trigger the next route. Each new operation starts
  with the first strategy. If the public pool is unavailable, other configured
  strategies remain available.
- Only `version: 1` is accepted. Unknown settings, other versions, an empty
  strategy list, duplicate strategies, and unknown strategies are rejected.
  The `--strategy` flag has been replaced by YAML configuration; `fallback`
  is no longer needed.

In all modes, `yt-dlp` receives an explicit route and does not inherit
`HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, or `NO_PROXY`. The UI displays `direct`
or the proxy address. In code, this policy is implemented by `DownloadStrategy`,
its `Direct`, `PersonalProxy`, and `RandomProxy` implementations, and the ordered
`Priority` chain in `internal/download/youtube/strategy.go`.

## Proxy checks

```sh
go run ./cmd/cli proxy test
```

Checks all personal addresses in `personal_proxy`, regardless of the enabled
strategies, followed by unexpired public addresses in `~/.meshvault/vault.db`.
Checks use HTTPS requests to YouTube with TLS verification: a five-second
timeout for personal proxies, a one-second timeout for public proxies, and up
to 1000 concurrent checks. Availability and check duration are displayed for
each address; usernames and passwords are hidden.

If at least one cached address works, source lists are not fetched and cache
expiry is not extended. If the cache is empty, expired, or contains no working
addresses, fresh ProxyScrape, Proxifly, and TheSpeedX lists are fetched and
checked, and working addresses are saved to the usual one-hour cache. Personal
addresses are not stored in the public cache. Check results do not change
`config.yaml` settings, although missing fields are added when the configuration
is loaded. This command does not download tracks and does not require
`yt-dlp` or `ffmpeg`. Log: `logs/proxy-test.jsonl`.

## YouTube dependencies

**Install `yt-dlp` and `ffmpeg` separately and make them available in `PATH`.**
Meshvault does not install them automatically. Check your installation with:

```sh
yt-dlp --version
ffmpeg -version
```

Use an up-to-date `yt-dlp`. Modern YouTube JavaScript challenges may also require
a supported JavaScript runtime, such as Deno, and `yt-dlp-ejs` components. See
the [yt-dlp instructions](https://github.com/yt-dlp/yt-dlp/wiki/EJS).

Search uses `ytsearch1:artist title` and selects the first YouTube result, which
is not guaranteed to match the original recording. `yt-dlp` downloads the best
audio stream (`bestaudio`, with up to five concurrent fragments), without
conversion. The source is cached at `download_dir/.sources/<video-id>/source`
in its original format. No extension is needed: FFmpeg detects the format from
the file contents. A separate local FFmpeg invocation then converts it to MP3
at 192 kbit/s. Conversion errors do not switch network routes; the source is
retained and reused on retries without downloading the audio again.

After successful conversion and verification that the MP3 is non-empty, the
source is deleted. Failed conversions retain their source for retries. Old
cached sources for previously completed MP3s are not automatically cleaned up.
Completed MP3s are still skipped without searching. User `yt-dlp` configuration
is ignored so it cannot change the format or output path.

The download display shows percentage, speed, and remaining time, followed by
the `Converting to MP3` status. The completed MP3 is copied to
`~/.meshvault/downloaded/Artist — Track title [video-id].mp3`, or the configured
`download_dir`, with size verification and SHA-256 recording. Artist and title
are taken from the YouTube search result; the artist may be the channel name.
Unsafe filename characters are replaced, long names are shortened, and the
video ID prevents collisions between identically named videos. Temporary
conversion files are removed after processing; sources are retained only if
conversion fails.

Before searching, the database's mapping between the original query and the
local file is checked. If a regular file exists and is larger than zero bytes,
both search and download are skipped. SHA-256 is recorded during downloads but
is no longer checked when deciding whether to skip. Missing and empty files
are downloaded again. The current output directory is respected: a file in
another directory does not prevent downloading to a new one. If every track
is already downloaded, neither proxy preparation nor installed `yt-dlp` and
`ffmpeg` binaries are needed.

For older downloads, query mappings are recovered from successful events in
`logs/download.jsonl`. If this information is unavailable, an old file may
require one search; the mapping is then saved for future runs. Old
`<video-id>.mp3` files are automatically renamed during the next track processing
if they are recorded in history and non-empty. No new download is needed, and
the hash is registered for the new path. Untracked old files are left untouched.

## Queue and diagnostics

- Searches and downloads share a worker pool. The default is 50 workers in
  `direct` mode, or `min(available personal and prepared public proxies, 50)`
  when proxy strategies have available addresses. `--workers N` replaces the
  default limit of 50 with a positive number. For example:
  `go run ./cmd/cli download --workers 20`.
  The selected worker count is displayed at startup and logged. Searches run
  concurrently without a one-second delay; only audio download starts are
  spaced at least one second apart.
- The first HTTP 429 pauses new requests for the affected stage across all
  workers. Download errors do not pause searches, and vice versa. Cooldowns
  increase in waves: 5, 10, 15, 20, 25, then 30 seconds maximum. Multiple 429s
  from already running requests count as one wave. Running operations are not
  interrupted. After a download pause, starts resume at one-second intervals;
  searches remain concurrent.
- Tracks receiving HTTP 429 stay queued with unlimited retries until processed
  or the application is stopped.
- Other technical errors receive five retries after 1, 2, 4, 8, and 16 seconds.
- Successful searches with no results are not retried and are displayed
  separately.
- Structured `zap` log: `logs/download.jsonl`.
- The terminal displays HTTP 429 as a search or download pause with a countdown
  to the automatic retry.
- Download history and query-to-file mappings are stored in
  `~/.meshvault/vault.db`, independently of Spotify synchronization.

Download only material for which you have the necessary rights.

## Tests

Regular tests do not require YouTube or installed external tools. To perform a
real search and download of the Rick Astley test video (`dQw4w9WgXcQ`), with MP3
verification using `ffprobe`:

```sh
MESHVAULT_YOUTUBE_LIVE=1 go test ./internal/download/youtube -run TestLiveYouTubeSearchAndDownload -v -count=1
```

This check also requires `ffprobe`, which is distributed with FFmpeg.
The downloaded test file is removed after the check.

## Free proxies

If `strategy` includes `random_proxy`, `download` prepares the public pool
before processing pending tracks. This step is not used for `direct` or
`personal_proxy`:

1. Read unexpired `verified_proxies` entries from `~/.meshvault/vault.db` if
   the cache was created by a complete check using the current sources and
   timeout. Old ProxyScrape-only caches or caches created with an eight-second
   timeout are refreshed on the first run. A fresh, complete cache skips source
   API requests and verification.
2. Otherwise, fetch **all available pages** of HTTP, SOCKS4, and SOCKS5 proxies
   from [ProxyScrape](https://docs.proxyscrape.com/api-reference/public-api/get-proxy-list).
   For each protocol, use `limit=2000` and `skip` until the last page is reached.
   The value 2000 is a page size, not a total pool limit. The `ssl=yes` filter
   has been removed; the application checks HTTPS support itself.
   Also fetch the complete [Proxifly list](https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/all/data.txt)
   of HTTP, HTTPS, SOCKS4, and SOCKS5 addresses, then merge and deduplicate them.
   Fetch `http.txt`, `socks4.txt`, and `socks5.txt` from
   [TheSpeedX/PROXY-List](https://github.com/TheSpeedX/PROXY-List), assigning the
   protocol indicated by each file. All lists are merged without duplicates.
   If a source or individual file is unavailable, use the remaining sources
   and log a warning. If no source returns addresses, pool preparation fails.
   Caches created without TheSpeedX are refreshed on the first run of the new
   version.
3. Check proxies with **1000 workers** using an HTTPS request to YouTube's
   `robots.txt` through each proxy. The overall timeout is **one second** for
   connection establishment, TLS, and receiving the response. TLS certificates
   are verified, and HTTP 200 is required. This checks YouTube access, not ICMP
   reachability, and does not guarantee that audio downloads will work.
   Working but slow proxies are excluded. Running 1000 simultaneous checks
   also requires sufficient open-file limits and network resources.
4. Save only successfully verified addresses for **one hour** from verification.
   Expired entries are removed when the cache is read.
5. For every search, download, and retry, pass the selected address to
   `yt-dlp --proxy`. Selection uses **LRU**: among available proxies, choose the
   one that has gone unused the longest; selection immediately updates its
   position. After HTTP 429, that address is excluded for 5, 10, 15, 20, 25,
   then at most 30 seconds on subsequent waves. The per-address cooldown is
   shared between search and download, and responses received during an
   existing cooldown do not extend it. Once the cooldown expires, the address
   re-enters LRU selection. If the whole pool is cooling down, the request waits
   for the earliest expiry and checks the pool again, within the overall
   operation timeout. Cooldown expiry is an estimate, not a guarantee that
   YouTube has lifted the block. LRU state and cooldowns exist only in memory
   for the current run.

Active search and download rows display the proxy selected for that connection,
for example, `proxy: socks5://203.0.113.1:1080`. The address is updated on retries
and not displayed before selection. In the JSON log, `request_started` includes
`job_id`, `attempt`, `stage`, `connection`, `connection_attempt`, and `proxy`
(`direct` for direct connections). Errors include the failed connection's
address. Usernames, passwords, and URL parameters are hidden.

If only one proxy is available, it is reused. Different proxy addresses do not
necessarily mean different exit IPs. The prepared pool is used for the current
run; the cache is checked again on the next run.

With `[random_proxy]`, the command exits with diagnostics if no valid proxies
are available or preparation fails. There is no silent fallback to a direct
connection. Inherited `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, and `NO_PROXY`
do not override the selected route in the child `yt-dlp` process. Existing
download spacing and HTTP 429 handling remain in effect.

Source lists are untrusted: local and reserved IP addresses are excluded, TLS
verification stays enabled, and Spotify cookies or tokens are never sent
through these proxies. A free proxy can stop working even within the cache's
one-hour lifetime; a successful probe does not guarantee stable speed or access
to YouTube media servers.

To perform a live API and proxy check using a temporary SQLite database rather
than the main library:

```sh
MESHVAULT_PROXY_LIVE=1 go test ./internal/proxy -run TestLiveProxyScrapePreparation -v -count=1
```
