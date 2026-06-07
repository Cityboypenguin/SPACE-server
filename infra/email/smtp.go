package email

import (
	"fmt"
	"net/smtp"
	"os"
)

type EmailService interface {
	Send(to, subject, body string) error
}

type SMTPEmailService struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSMTPEmailService() *SMTPEmailService {
	return &SMTPEmailService{
		host:     os.Getenv("SMTP_HOST"),
		port:     os.Getenv("SMTP_PORT"),
		username: os.Getenv("SMTP_USERNAME"),
		password: os.Getenv("SMTP_PASSWORD"),
		from:     os.Getenv("SMTP_FROM"),
	}
}

func (s *SMTPEmailService) Send(to, subject, body string) error {
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		s.from, to, subject, body,
	))

	addr := s.host + ":" + s.port
	if err := smtp.SendMail(addr, auth, s.from, []string{to}, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}
