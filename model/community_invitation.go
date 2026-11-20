package model

import "time"

const (
	CommunityInvitationPending  = "pending"
	CommunityInvitationAccepted = "accepted"
	CommunityInvitationDeclined = "declined"
	CommunityInvitationRevoked  = "revoked"
	CommunityInvitationExpired  = "expired"
	AuthorRoleCommunityWelcome  = "COMMUNITY_WELCOME"
)

type CommunityInvitation struct {
	ID          int64
	RoomID      int64
	SenderID    int64
	RecipientID *int64
	State       string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

func (i *CommunityInvitation) Status(now time.Time) string {
	if i.State == CommunityInvitationPending && !now.Before(i.ExpiresAt) {
		return CommunityInvitationExpired
	}
	return i.State
}

type CommunityJoinStampSettings struct {
	WelcomeIDs  []string
	GreetingIDs []string
}
