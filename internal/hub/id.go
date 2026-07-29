package hub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewID returns a lexicographically-sortable id: a millisecond timestamp
// prefix followed by random bytes. Sortable IDs are what let the replay
// buffer answer "everything after id X" with a plain string comparison.
func NewID() string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%013d-%s", time.Now().UnixMilli(), hex.EncodeToString(buf))
}
