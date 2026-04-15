package session

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

func newID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102-150405")
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}
