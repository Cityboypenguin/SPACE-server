package model

import "time"

const (
	CommunityInvitationPending  = "pending"
	CommunityInvitationAccepted = "accepted"
	CommunityInvitationDeclined = "declined"
	CommunityInvitationRevoked  = "revoked"
	CommunityInvitationExpired  = "expired"
	AuthorRoleCommunityWelcome   = "COMMUNITY_WELCOME"
)

type CommunityInvitation struct {
	ID          int64
	RoomID      int64
	SenderID    int64
	RecipientID *int64
	State       string
	// Message は招待に添えた一言（任意）。
	Message *string
	// UseCount はこの招待から参加した人数。
	UseCount    int
	CreatedAt   time.Time
	ExpiresAt   time.Time
	RespondedAt *time.Time
}

func (i *CommunityInvitation) Status(now time.Time) string {
	if i.State != CommunityInvitationPending {
		return i.State
	}
	if !now.Before(i.ExpiresAt) {
		return CommunityInvitationExpired
	}
	return i.State
}

// 招待の作成時に選べる値。
const (
	CommunityInvitationMessageMaxLength = 200
)

// CommunityInviteOptions は招待を作るときの任意の設定。
type CommunityInviteOptions struct {
	Message *string
	// ExpiresIn は有効期間（0 は既定の7日）。
	ExpiresIn time.Duration
}

// 招待候補（検索結果）の状態。
const (
	CommunityCandidateAvailable   = "available"
	CommunityCandidateMember      = "member"
	CommunityCandidateInvited     = "invited"
	CommunityCandidateUnavailable = "unavailable"
)

type CommunityJoinStampSettings struct {
	WelcomeIDs  []string
	GreetingIDs []string
}

type CommunityWelcomeTarget struct {
	MessageID int64
	UserName  string
}
