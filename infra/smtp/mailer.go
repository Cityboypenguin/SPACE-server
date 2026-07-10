package smtp

import (
	"context"
	"fmt"
	"net/smtp"
	"os"
	"strconv"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

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
		port = 587
	}
	return &SMTPMailer{
		host:     os.Getenv("SMTP_HOST"),
		port:     port,
		username: os.Getenv("SMTP_USERNAME"),
		password: os.Getenv("SMTP_PASSWORD"),
		from:     os.Getenv("SMTP_FROM"),
	}
}

func (m *SMTPMailer) SendPasswordResetLink(_ context.Context, toEmail, resetLink string) error {
	auth := smtp.PlainAuth("", m.username, m.password, m.host)
	addr := fmt.Sprintf("%s:%d", m.host, m.port)

	subject := "パスワードリセットリンク"
	body := fmt.Sprintf("以下のリンクからパスワードを再設定してください。\n\n%s\n\nこのリンクは15分間有効です。\n心当たりがない場合は無視してください。", resetLink)
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.from, toEmail, subject, body,
	))

	return smtp.SendMail(addr, auth, m.from, []string{toEmail}, msg)
}
