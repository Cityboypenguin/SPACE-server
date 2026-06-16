package model

import "time"

type InquiryStatus string

const (
	InquiryStatusPending    InquiryStatus = "PENDING"
	InquiryStatusInProgress InquiryStatus = "IN_PROGRESS"
	InquiryStatusResolved   InquiryStatus = "RESOLVED"
)

type InquiryCategory string

const (
	InquiryCategoryDM        InquiryCategory = "DM"
	InquiryCategoryPost      InquiryCategory = "POST"
	InquiryCategoryCommunity InquiryCategory = "COMMUNITY"
	InquiryCategoryPassword  InquiryCategory = "PASSWORD"
	InquiryCategoryLogin     InquiryCategory = "LOGIN"
	InquiryCategoryOther     InquiryCategory = "OTHER"
)

type Inquiry struct {
	ID        string
	Name      string
	Email     string
	Category  InquiryCategory
	Subject   string
	Content   string
	Status    InquiryStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}
