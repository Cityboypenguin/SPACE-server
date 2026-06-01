package model

import "time"

type InquiryStatus string

const (
	InquiryStatusPending    InquiryStatus = "PENDING"
	InquiryStatusInProgress InquiryStatus = "IN_PROGRESS"
	InquiryStatusResolved   InquiryStatus = "RESOLVED"
)

type Inquiry struct {
	ID        string
	Name      string
	Email     string
	Subject   string
	Content   string
	Status    InquiryStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}
