package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "auth",
	Long:  `auth long`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("auth")
	},
}
