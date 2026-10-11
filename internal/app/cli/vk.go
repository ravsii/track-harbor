package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/term"
	"trackharbor/internal/db"
	"trackharbor/internal/download"
	"trackharbor/internal/repository"
	"trackharbor/internal/source/vk"
)

var vkCmd = &cobra.Command{Use: "vk", Short: "Save a VK user token and sync My Music"}

func init() {
	vkCmd.AddCommand(vkAuthCmd, vkSyncCmd)
	rootCmd.AddCommand(vkCmd)
}

var vkAuthCmd = &cobra.Command{
	Use: "auth", Short: "Save a manually obtained VK user access token (VFeed)",
	Args: cobra.NoArgs, SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "Obtain a user token using VFeed at https://vkhost.github.io/ (application ID 4083558).")
		fmt.Fprintln(cmd.OutOrStdout(), "Review the requested permissions and enter your password only on VK's login page.")
		fmt.Fprintln(cmd.OutOrStdout(), "Paste the access token or final redirect URL. Do not share it; audio access is checked by vk sync.")
		token, err := readVKToken(cmd.InOrStdin(), cmd.OutOrStdout())
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

func readVKToken(input io.Reader, out io.Writer) (string, error) {
	var text string
	if f, ok := input.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		value, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read VK token: input failed")
		}
		text = string(value)
	} else {
		scanner := bufio.NewScanner(input)
		if !scanner.Scan() {
			return "", fmt.Errorf("read VK token: no input or input too large")
		}
		text = scanner.Text()
	}
	text = strings.TrimSpace(text)
	if strings.Contains(text, "access_token=") {
		fragment := text
		if i := strings.IndexByte(fragment, '#'); i >= 0 {
			fragment = fragment[i+1:]
		}
		if i := strings.IndexByte(fragment, '?'); i >= 0 {
			fragment = fragment[i+1:]
		}
		params, err := url.ParseQuery(strings.TrimPrefix(fragment, "#"))
		if err != nil {
			return "", fmt.Errorf("invalid VK token URL")
		}
		text = params.Get("access_token")
	}
	if text == "" || strings.ContainsAny(text, " \t\r\n:/?#&=") {
		return "", fmt.Errorf("paste a non-empty VK access token or redirect URL containing access_token")
	}
	return text, nil
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
		songs := make([]repository.Song, 0, len(tracks))
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
			songs = append(songs, repository.Song{Artists: string(artists), Title: title, CreatedAt: time.Now()})
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
