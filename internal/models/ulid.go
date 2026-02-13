package models

import (
	"crypto/rand"

	"github.com/oklog/ulid/v2"
)

// NewID generates a new ULID string.
func NewID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}
