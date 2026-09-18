package model

const (
	RoomUserRoleOwner  = "owner"
	RoomUserRoleMember = "member"
)

type RoomMember struct {
	User *User
	Role string
}
