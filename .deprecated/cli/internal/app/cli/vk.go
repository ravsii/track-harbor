package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"trackharbor/cli/internal/db"
	"trackharbor/cli/internal/download"
	"trackharbor/cli/internal/repository"
	"trackharbor/vk"
)

var vkCmd = &cobra.Command{Use: "vk", Short: "Save a VK user token and sync My Music"}

func init() {
	vkCmd.AddCommand(vkAuthCmd, vkSyncCmd)
	rootCmd.AddCommand(vkCmd)
}

var vkAuthCmd = &cobra.Command{
	Use: "auth", Short: "Open vk.com application authorization in a browser and save a VK user token",
	Args: cobra.NoArgs, SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		authURL := vk.AuthorizationURL()
		fmt.Fprintln(cmd.OutOrStdout(), "Opening VK authorization for vk.com (application ID 6287487; audio and offline permissions)…")
		if err := exec.CommandContext(cmd.Context(), "open", authURL).Run(); err != nil {
			if cmd.Context().Err() != nil {
				return cmd.Context().Err()
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "Could not open the browser automatically. Open this URL manually:")
			fmt.Fprintln(cmd.ErrOrStderr(), authURL)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Review the requested permissions and enter your password only on VK's login page.")
		fmt.Fprintln(cmd.OutOrStdout(), "This flow returns to VK's blank.html, not a local callback. Copy the final address after authorization.")
		fmt.Fprintln(cmd.OutOrStdout(), "Paste the access token or final redirect URL. Do not share it; audio access is checked by vk sync.")
		token, err := readVKToken(cmd.InOrStdin())
		if err != nil {
			return err
		}
		database, err := db.New()
		if err != nil {
			return err
		}
		defer database.Close()
		repo, err := repository.NewVKRepository(database)
		if err != nil {
			return err
		}
		if err := repo.SaveToken(token); err != nil {
			return fmt.Errorf("save VK token: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "VK token saved in ~/.trackharbor/vault.db. Run trackharbor vk sync to import My Music.")
		return nil
	},
}

func readVKToken(input io.Reader) (string, error) {
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return "", fmt.Errorf("read VK token: no input or input too large")
	}
	return vk.ParseToken(scanner.Text())
}

var vkSyncCmd = &cobra.Command{
	Use: "sync", Short: "Sync My Music into a separate VK library; Spotify tracks are preserved",
	Args: cobra.NoArgs, SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		logger, closeLog, err := download.NewFileLogger("logs/vk-sync.jsonl")
		if err != nil {
			return err
		}
		defer func() {
			if runErr != nil {
				logger.Error("vk_sync_failed", zap.Error(runErr))
			}
			if err := closeLog(); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "VK sync log could not be closed.")
			}
		}()
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "Log: logs/vk-sync.jsonl")
		logger.Info("vk_sync_started")
		database, err := db.New()
		if err != nil {
			return err
		}
		defer database.Close()
		repo, err := repository.NewVKRepository(database)
		if err != nil {
			return err
		}
		token, err := repo.LoadToken()
		if err != nil {
			return err
		}
		if token == "" {
			return fmt.Errorf("no saved VK token; run trackharbor vk auth first")
		}
		fmt.Fprintln(out, "[1/3] Fetching My Music from VK…")
		client := vk.NewClient(token)
		defer client.Close()
		tracks, err := client.MyMusic(cmd.Context(), func(received, total int) {
			fmt.Fprintf(out, "      Received %d of %d tracks.\n", received, total)
			logger.Info("vk_page_received", zap.Int("received", received), zap.Int("total", total))
		})
		if err != nil {
			return fmt.Errorf("fetch VK My Music (previous libraries preserved): %w", err)
		}
		songs := make([]repository.VKSong, 0, len(tracks))
		skipped := 0
		for _, track := range tracks {
			artist, title := strings.TrimSpace(track.Artist), strings.TrimSpace(track.Title)
			if artist == "" || title == "" {
				skipped++
				continue
			}
			artists, err := json.Marshal([]string{artist})
			if err != nil {
				return fmt.Errorf("encode VK artist: %w", err)
			}
			var addedAt time.Time
			if track.Date > 0 {
				addedAt = time.Unix(track.Date, 0)
			}
			songs = append(songs, repository.VKSong{
				Song:    repository.Song{Artists: string(artists), Title: title, CreatedAt: addedAt},
				OwnerID: track.OwnerID, AudioID: track.ID, URL: track.URL, CoverURL: track.Album.Thumb.Photo600,
			})
			if songs[len(songs)-1].CoverURL == "" {
				songs[len(songs)-1].CoverURL = track.Album.Thumb.Photo300
			}
		}
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		fmt.Fprintln(out, "[2/3] Updating the separate VK library (Spotify tracks are untouched)…")
		saved, err := repo.ReplaceAll(songs)
		if err != nil {
			return fmt.Errorf("save VK tracks (previous library preserved): %w", err)
		}
		logger.Info("vk_tracks_saved", zap.Int("saved", saved), zap.Int("skipped", skipped), zap.Int("duplicates", len(songs)-saved))
		fmt.Fprintf(out, "[3/3] Done. Saved %d VK tracks; skipped %d incomplete entries and %d duplicates.\n", saved, skipped, len(songs)-saved)
		return nil
	},
}
