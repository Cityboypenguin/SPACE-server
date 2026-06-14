package post

import (
	"fmt"
	"unicode/utf8"
)

const maxPostContentChars = 500

func validatePostContent(content string) error {
	if utf8.RuneCountInString(content) > maxPostContentChars {
		return fmt.Errorf("post content must be %d characters or less", maxPostContentChars)
	}
	return nil
}
