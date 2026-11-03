package model

import "strings"

type MessageReaction struct {
	Emoji       string
	Count       int
	ReactedByMe bool
}

const StampIDPrefix = "stamp:"

// IsValidMessageStampID checks storage syntax only. Use cases check the catalog.
func IsValidMessageStampID(id string) bool {
	if !strings.HasPrefix(id, StampIDPrefix) || len(id) <= len(StampIDPrefix) || len(id) > 64 {
		return false
	}
	for _, c := range strings.TrimPrefix(id, StampIDPrefix) {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
