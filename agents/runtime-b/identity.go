package runtimeb

import (
	"crypto/sha256"
	"encoding/hex"
)

func derivedID(domain, messageID string) string {
	digest := sha256.Sum256([]byte(domain + "\x00" + messageID))
	return "runtime-b-" + domain + "-" + hex.EncodeToString(digest[:])
}
