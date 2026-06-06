package model

import "time"

type TermsOfService struct {
	ID            int64
	Version       string
	ObjectKey     string
	EffectiveDate time.Time
	CreatedAt     time.Time
}

type TermsConsent struct {
	ID          int64
	UserID      int64
	TermsID     int64
	ConsentedAt time.Time
}
