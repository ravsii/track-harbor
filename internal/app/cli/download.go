package cli

import "github.com/spf13/cobra"

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Скачать все треки из базы",
	Long:  "Скачать все треки из meshtune.db в tmp_music через YouTube. Параллельность фиксирована: 10 загрузок.",
	// Keep the existing bare-command behavior and the explicit subcommand
	// on the same download path (including the fixed 10-worker pool).
	Run: rootCmd.Run,
}

func init() {
	downloadCmd.Flags().Bool("pprof", false, "Save cpu+mem profile to meshvault_{cpu,mem}.pprof")
	downloadCmd.Flags().Bool("allocs", false, "Save allocs profile to meshvault_allocs.pprof")
	downloadCmd.Flags().String("provider", "youtube", "Источник скачивания (доступен только youtube)")
	rootCmd.AddCommand(downloadCmd)
}
