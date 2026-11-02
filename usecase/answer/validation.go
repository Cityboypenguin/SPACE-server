package answer

import (
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

const MaxBodyChars = 1000

func validateBody(body string) error {
	if utf8.RuneCountInString(body) > MaxBodyChars {
		return apperr.InvalidInput("回答は1000文字以内で入力してください")
	}
	return nil
}
