package transport

import "strings"

func maskCommand(cmd string) string {
	commandsToMask := []string{"cat", "sed", "awk", "grep"}

	// Checked before the verb list so the redirect target survives
	if masked, ok := maskHeredoc(cmd); ok {
		return masked
	}

	for _, mask := range commandsToMask {
		if strings.HasPrefix(cmd, mask) {
			return mask + " <REDACTED>"
		}
	}

	return cmd
}

// A heredoc body is how file content — and any credential in it — reaches a
// command, whatever verb the command happens to start with.
func maskHeredoc(cmd string) (string, bool) {
	if !strings.Contains(cmd, "<< '") {
		return "", false
	}

	opener, _, found := strings.Cut(cmd, "\n")
	if !found {
		return "", false
	}

	return opener + " <REDACTED>", true
}
