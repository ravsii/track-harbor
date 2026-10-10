package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// Without a subcommand Cobra displays help; it does not start a download.
var rootCmd = &cobra.Command{
	Use:   "meshvault",
	Short: "Синхронизация Spotify и скачивание музыки с YouTube",
}

func Execute() {
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(syncCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
