package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"trackharbor/cli/internal/db"
	"trackharbor/cli/internal/download"
	"trackharbor/cli/internal/repository"
	"trackharbor/spotify"
)

var spotifyCmd = &cobra.Command{
	Use:   "spotify",
	Short: "Authenticate with Spotify and sync Liked Songs",
}

func init() {
	spotifyCmd.AddCommand(authCmd, syncCmd)
	rootCmd.AddCommand(spotifyCmd)
}

var authCmd = &cobra.Command{
	Use:          "auth",
	Short:        "Save Spotify Web Player request tokens",
	Long:         "Paste authorization and client-token headers from Spotify Web Player. Existing tokens are replaced in ~/.trackharbor/vault.db; other headers and cookies are not stored.",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		headers, err := readSyncHeaders(cmd.InOrStdin(), cmd.OutOrStdout())
		if err != nil {
			return err
		}
		database, err := db.New()
		if err != nil {
			return err
		}
		defer database.Close()
		repo := repository.NewSpotifyHeadersRepository(database)
		if _, err := repo.Load(); err != nil {
			return err
		}
		if err := repo.Save(*headers); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Spotify tokens saved. Run trackharbor spotify sync to verify them and import Liked Songs.")
		return nil
	},
}

var syncCmd = &cobra.Command{
	Use:          "sync",
	Short:        "Discover and sync Spotify Liked Songs using saved tokens",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		started := time.Now()
		out := cmd.OutOrStdout()
		logger, closeLog, err := download.NewFileLogger("logs/spotify-sync.jsonl")
		if err != nil {
			return fmt.Errorf("create Spotify sync log: %w", err)
		}
		fmt.Fprintln(out, "Log: logs/spotify-sync.jsonl")
		logger.Info("spotify_sync_started")
		defer func() {
			if runErr != nil {
				logger.Error("spotify_sync_failed", zap.Error(runErr), zap.Duration("duration", time.Since(started)))
			} else {
				logger.Info("spotify_sync_finished", zap.Duration("duration", time.Since(started)))
			}
			if err := closeLog(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Spotify sync log error: %v\n", err)
			}
		}()
		fmt.Fprintln(out, "[1/4] Checking saved headers in ~/.trackharbor/vault.db…")
		database, err := db.New()
		if err != nil {
			return fmt.Errorf("[1/4] open database: %w", err)
		}
		defer database.Close()
		if err := database.Ping(); err != nil {
			return fmt.Errorf("[1/4] open database: %w", err)
		}
		headerRepo := repository.NewSpotifyHeadersRepository(database)
		headers, err := headerRepo.Load()
		if err != nil {
			return fmt.Errorf("[1/4] read saved headers: %w", err)
		}

		if headers == nil {
			return fmt.Errorf("no saved Spotify tokens; run trackharbor spotify auth first")
		}
		logger.Info("spotify_saved_headers_loaded", zap.Bool("authorization_present", headers.Authorization != ""), zap.Bool("client_token_present", headers.ClientToken != ""))
		fmt.Fprintln(out, "[2/4] Discovering Liked Songs in your Spotify library…")
		info, err := spotify.FindLikedSongs(headers.Authorization, headers.ClientToken, logger)
		if err != nil {
			return fmt.Errorf("[2/4] discover Liked Songs: %w; see logs/spotify-sync.jsonl", err)
		}
		playlistURI := info.Data.PlaylistV2.URI

		playlist := info.Data.PlaylistV2
		total := playlist.Content.TotalCount
		fmt.Fprintf(out, "      Playlist: %q.\n", playlist.Name)
		fmt.Fprintf(out, "      Page 1 (offset=0): received %d of %d tracks.\n", len(playlist.Content.Items), total)
		if total < 0 || len(playlist.Content.Items) > total || len(playlist.Content.Items) == 0 && total > 0 {
			return fmt.Errorf("[2/4] first page contains an incomplete or invalid track list")
		}
		for pageNumber := 2; len(playlist.Content.Items) < total; pageNumber++ {
			offset := len(playlist.Content.Items)
			fmt.Fprintf(out, "      Fetching page %d (offset=%d)…\n", pageNumber, offset)
			next, err := spotify.GetPlaylistPage(headers.Authorization, headers.ClientToken, playlistURI, offset, logger)
			if err != nil {
				return fmt.Errorf("[2/4] page %d (offset=%d): %w", pageNumber, offset, err)
			}
			content := next.Data.PlaylistV2.Content
			if content.TotalCount != total {
				return fmt.Errorf("[2/4] track count changed during import; run sync again")
			}
			if content.PagingInfo.Offset != offset {
				return fmt.Errorf("[2/4] page %d: Spotify returned offset %d instead of %d", pageNumber, content.PagingInfo.Offset, offset)
			}
			if len(content.Items) == 0 || len(content.Items) > total-offset {
				return fmt.Errorf("[2/4] page %d: incomplete or invalid track list", pageNumber)
			}
			playlist.Content.Items = append(playlist.Content.Items, content.Items...)
			fmt.Fprintf(out, "      Page %d (offset=%d): received %d of %d tracks.\n", pageNumber, offset, len(playlist.Content.Items), total)
		}
		fmt.Fprintf(out, "      Received %d of %d tracks.\n", len(playlist.Content.Items), total)
		var ss []repository.Song
		unavailable := 0

		for i, song := range playlist.Content.Items {
			title := song.ItemV3.Data.IdentityTrait.Name
			artists := make([]string, 0, len(song.ItemV3.Data.IdentityTrait.Contributors.Items))

			for _, c := range song.ItemV3.Data.IdentityTrait.Contributors.Items {
				if c.Name != "" {
					artists = append(artists, c.Name)
				}
			}
			if title == "" {
				title = song.ItemV2.Data.Name
				artists = artists[:0]
				for _, artist := range song.ItemV2.Data.Artists.Items {
					if artist.Profile.Name != "" {
						artists = append(artists, artist.Profile.Name)
					}
				}
			}
			if strings.TrimSpace(title) == "" {
				unavailable++
				fmt.Fprintf(out, "      %d. Unavailable track — skipped.\n", i+1)
				continue
			}

			var buf bytes.Buffer

			err := json.NewEncoder(&buf).Encode(artists)
			if err != nil {
				return fmt.Errorf("[2/4] prepare track %d: %w", i+1, err)
			}
			artistName := strings.Join(artists, ", ")
			if artistName == "" {
				artistName = "Unknown artist"
			}
			fmt.Fprintf(out, "      %d. %s — %s\n", i+1, artistName, title)
			addedAt, err := time.Parse(time.RFC3339Nano, song.AddedAt.ISOString)
			if err != nil {
				// Older entries may not include a date; keep it unknown for storage.
				addedAt = time.Time{}
			}

			ss = append(ss, repository.Song{
				Artists:   buf.String(),
				Title:     title,
				CreatedAt: addedAt,
			})
		}

		fmt.Fprintln(out, "[3/4] Replacing the track list in ~/.trackharbor/vault.db…")
		logger.Info("spotify_tracks_prepared", zap.Int("tracks", len(ss)), zap.Int("unavailable", unavailable))
		repo := repository.NewSongRepository(database)
		saved, err := repo.ReplaceAll(ss)
		if err != nil {
			return fmt.Errorf("[3/4] save tracks (previous list preserved): %w", err)
		}
		fmt.Fprintf(out, "[4/4] Done. Saved %d tracks in %.1f s.\n", saved, time.Since(started).Seconds())
		logger.Info("spotify_tracks_saved", zap.Int("saved", saved), zap.Int("duplicates", len(ss)-saved), zap.Int("unavailable", unavailable))
		if repeats := len(ss) - saved; repeats > 0 {
			fmt.Fprintf(out, "      Duplicates: %d (one copy kept per track).\n", repeats)
		}
		if unavailable > 0 {
			fmt.Fprintf(out, "      Unavailable tracks skipped: %d.\n", unavailable)
		}
		return nil
	},
}

func readSyncHeaders(input io.Reader, out io.Writer) (*repository.SpotifyHeaders, error) {
	fmt.Fprintln(out, "Paste Spotify request headers, not cookies.")
	fmt.Fprintln(out, "      Format (replace the values with your own):")
	fmt.Fprintln(out, "      authorization: Bearer <token>")
	fmt.Fprintln(out, "      client-token: <client token>")
	fmt.Fprintln(out, "      Finish with a blank line (press Enter again) or Ctrl+D.")
	hMap, skipped, err := parseHeaders(input)
	if err != nil {
		return nil, fmt.Errorf("read headers: %w", err)
	}
	fmt.Fprintf(out, "      Read %d headers.\n", len(hMap))
	if skipped > 0 {
		fmt.Fprintf(out, "      Invalid lines skipped: %d.\n", skipped)
	}
	presence := func(value string) string {
		if value == "" {
			return "missing"
		}
		return "present"
	}
	fmt.Fprintf(out, "      authorization: %s; client-token: %s\n", presence(hMap["authorization"]), presence(hMap["client-token"]))
	if hMap["authorization"] == "" || hMap["client-token"] == "" {
		return nil, fmt.Errorf("non-empty authorization and client-token headers from a Spotify Web Player request are required; a Cookie header cannot replace them")
	}
	return &repository.SpotifyHeaders{Authorization: hMap["authorization"], ClientToken: hMap["client-token"]}, nil
}

func parseHeaders(input io.Reader) (map[string]string, int, error) {
	scanner := bufio.NewScanner(input)

	headerMap := make(map[string]string)
	skipped := 0

	for scanner.Scan() {
		text := scanner.Text()
		if strings.TrimSpace(text) == "" {
			break
		}
		headers := strings.SplitN(text, ":", 2)
		if len(headers) != 2 {
			skipped++
			continue
		}
		key, value := strings.ToLower(strings.TrimSpace(headers[0])), strings.TrimSpace(headers[1])
		// HTTP header names are ASCII tokens. In particular, a ':' inside
		// a cookie URL does not turn the cookie string into a header.
		invalidName := strings.IndexFunc(key, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r))
		}) >= 0
		if key == "" || invalidName {
			skipped++
			continue
		}
		headerMap[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, skipped, err
	}

	return headerMap, skipped, nil
}
