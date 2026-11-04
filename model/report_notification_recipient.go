package model

import (
	"net/mail"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

// maxNotificationEmailLength は report_notification_recipients.email の列長。
const maxNotificationEmailLength = 255

// ReportNotificationRecipient は通報のお知らせメールの宛先。
type ReportNotificationRecipient struct {
	ID        int64
	Email     string
	CreatedAt time.Time
}

// NormalizeNotificationEmail は宛先として保存する形へそろえて検証する。
// 利用者の登録と違って学籍番号形式には縛らない（教職員や共有アドレスも宛先になるため）。
// 「名前 <addr>」の形を通すと To ヘッダーへ余計な文字が入るので、アドレスだけを認める。
func NormalizeNotificationEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(normalized)
	if err != nil || addr.Address != normalized || len(normalized) > maxNotificationEmailLength {
		return "", apperr.InvalidInput("メールアドレスの形式が正しくありません")
	}
	return normalized, nil
}
