package transport

import "strings"

func maskCommand(cmd string) string {
	commandsToMask := []string{"cat", "sed", "awk", "grep"}

	for _, mask := range commandsToMask {
		if strings.HasPrefix(cmd, mask) {
			return mask + " <REDACTED>"
		}
	}

	return cmd
}
