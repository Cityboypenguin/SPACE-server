// Package smtp は repository.Mailer の唯一の実装を提供する。
// SMTP_* 環境変数の読み取りもここ1箇所に閉じる。
package smtp

import (
	"context"
	"fmt"
	"net/smtp"
	"os"
	"strconv"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// defaultSMTPPort は SMTP_PORT 未設定時のポート（submission over STARTTLS）。
const defaultSMTPPort = 587

type SMTPMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTPMailer() repository.Mailer {
	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if port == 0 {
		port = defaultSMTPPort
	}
	return &SMTPMailer{
		host:     os.Getenv("SMTP_HOST"),
		port:     port,
		username: os.Getenv("SMTP_USERNAME"),
		password: os.Getenv("SMTP_PASSWORD"),
		from:     os.Getenv("SMTP_FROM"),
	}
}

// Send は1通を送る。件名・本文は呼び出し側（ユースケース）が組み立てる。
func (m *SMTPMailer) Send(_ context.Context, to, subject, body string) error {
	auth := smtp.PlainAuth("", m.username, m.password, m.host)
	addr := fmt.Sprintf("%s:%d", m.host, m.port)

	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.from, to, subject, body,
	))

	if err := smtp.SendMail(addr, auth, m.from, []string{to}, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}
