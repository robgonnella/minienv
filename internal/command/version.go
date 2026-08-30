package command

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const VERSION = "0.1.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints version info",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info().Str("version", VERSION).Send()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
