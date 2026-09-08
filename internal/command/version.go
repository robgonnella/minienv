package command

import (
	"github.com/robgonnella/minienv/internal"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints version info",
		Run: func(_ *cobra.Command, _ []string) {
			log.Info().Str("version", internal.Version).Send()
		},
	}
}
