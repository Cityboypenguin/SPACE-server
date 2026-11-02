package model

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

const (
	// MaxAddendumChars は追記1件の文字数の上限（質問・回答の本文と同じ）。
	MaxAddendumChars = 1000
	// MaxAddendaPerTarget は1つの質問・回答に足せる追記の数の上限。
	// 細かい仕様は未決（仕様書 Q-09）なので、荒らしを防ぐための仮の数。
	MaxAddendaPerTarget = 10
)

// Addendum は質問または回答への追記1件。元の本文は変えず、追記用の枠に別に持つ。
// TargetID は、質問への追記なら質問のID、回答への追記なら回答のID。
type Addendum struct {
	ID        int64
	TargetID  int64
	UserID    int64
	Body      string
	CreatedAt time.Time
}

// NewAddendum は追記の本文を検証して作る（前後の空白は取る）。画像は付けられない。
func NewAddendum(targetID, userID int64, body string, now time.Time) (*Addendum, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, apperr.InvalidInput("追記を入力してください")
	}
	if utf8.RuneCountInString(body) > MaxAddendumChars {
		return nil, apperr.InvalidInput("追記は1000文字以内で入力してください")
	}
	return &Addendum{TargetID: targetID, UserID: userID, Body: body, CreatedAt: now}, nil
}
