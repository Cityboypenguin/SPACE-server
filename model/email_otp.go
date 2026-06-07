package model

import "time"

type EmailOTP struct {
	Email     string
	Code      string
	ExpiresAt time.Time
}
