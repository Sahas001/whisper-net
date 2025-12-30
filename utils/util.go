// Package utils provides utility functions for generating unique room IDs.
package utils

import (
	"crypto/rand"
	"encoding/hex"
)

func GenerateRoomID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "default"
	}
	return hex.EncodeToString(b)
}
