package command

import (
	"github.com/robgonnella/minienv/internal"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints version info",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info().Str("version", internal.VERSION).Send()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
