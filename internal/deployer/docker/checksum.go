package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
)

// The token is folded in so rotating it also replaces the agent.
func ngrokConfigChecksum(configContent []byte, authToken string) string {
	sum := sha256.New()

	writeChecksumParts(sum, string(configContent), authToken)

	return hex.EncodeToString(sum.Sum(nil))
}

// Length-prefixed so different splits of the same bytes cannot hash alike.
func writeChecksumParts(sum hash.Hash, parts ...string) {
	for _, part := range parts {
		_, _ = fmt.Fprintf(sum, "%d:%s", len(part), part)
	}
}
