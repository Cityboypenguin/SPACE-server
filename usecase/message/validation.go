package message

import (
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

const MaxContentChars = 2000

func validateContent(content string) error {
	if utf8.RuneCountInString(content) > MaxContentChars {
		return apperr.InvalidInput("メッセージは2000文字以内で入力してください")
	}
	return nil
}
