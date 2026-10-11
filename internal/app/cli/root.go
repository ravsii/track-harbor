package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// Without a subcommand Cobra displays help; it does not start a download.
var rootCmd = &cobra.Command{
	Use:   "meshvault",
	Short: "Sync Spotify playlists and download music from YouTube",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
