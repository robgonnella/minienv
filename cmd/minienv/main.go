// Package main is the minienv entry point.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/robgonnella/minienv/internal/command"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Called from main rather than init so importing this package from a test
// binary installs no global logger.
func setupLogging() {
	log.Logger = zerolog.New(os.Stdout).Output(zerolog.ConsoleWriter{
		Out:             os.Stderr,
		FormatTimestamp: func(_ any) string { return "" },
		FormatLevel: func(i any) string {
			formatWithColor := func(color int) string {
				return fmt.Sprintf("\033[%dmminienv\033[0m", color)
			}

			level := zerolog.DebugLevel
			levelStr, ok := i.(string)

			if !ok {
				return formatWithColor(zerolog.LevelColors[zerolog.DebugLevel])
			}

			// Remapped so minienv gets its own color scheme rather than
			// zerolog's defaults.
			switch strings.ToLower(levelStr) {
			case strings.ToLower(zerolog.TraceLevel.String()):
				level = zerolog.DebugLevel
			case strings.ToLower(zerolog.DebugLevel.String()):
				level = zerolog.InfoLevel
			case strings.ToLower(zerolog.InfoLevel.String()):
				level = zerolog.TraceLevel
			case strings.ToLower(zerolog.WarnLevel.String()):
				level = zerolog.WarnLevel
			case strings.ToLower(zerolog.ErrorLevel.String()):
				level = zerolog.ErrorLevel
			case strings.ToLower(zerolog.FatalLevel.String()):
				level = zerolog.FatalLevel
			}

			color := zerolog.LevelColors[level]

			return formatWithColor(color)
		},
	})
}

func main() {
	setupLogging()
	os.Exit(command.Execute())
}
