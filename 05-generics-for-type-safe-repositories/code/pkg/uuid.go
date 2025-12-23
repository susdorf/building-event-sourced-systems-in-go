package pkg

import (
	"crypto/rand"
	"fmt"
)

// NewUUID generates a simple UUID v4 for demo purposes.
func NewUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
