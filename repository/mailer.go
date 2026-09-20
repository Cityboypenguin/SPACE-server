package repository

import "context"

// Mailer は送信メール1通ぶんのポート。宛先・件名・本文だけを受け取り、SMTP の
// 認証やメッセージ組み立ては実装（infra/smtp）に閉じる。
//
// 以前は「汎用の Send を持つ infra/email.EmailService」と「用途専用の
// SendPasswordResetOTP を持つ repository.Mailer」の2実装が並立し、認証・
// メッセージ生成・送信がそれぞれ別に書かれていた。件名と本文は用途ごとの
// 文言＝ユースケースの関心なので、ポートは汎用の Send 1本に統一してある。
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}
