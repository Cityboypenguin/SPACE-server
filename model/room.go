package model

import "time"

const (
	RoomTypeCommunity = "community"
	RoomTypeDM        = "dm"
	RoomTypeCourse    = "course"
)

type Room struct {
	ParentRoomID    *int64
	ThreadCreatorID *int64
	ThreadClosedAt  *int64
	ID              int64
	Name            string
	Type            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateRoomParam struct {
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (r *Room) CreateRoom(param CreateRoomParam) {
	r.Name = param.Name
	r.Type = RoomTypeCommunity
	r.CreatedAt = param.CreatedAt
	r.UpdatedAt = param.UpdatedAt
}

// PolicyRoomID is the room whose membership, course rules and identity apply.
// Threads keep their own ID for messages, subscriptions and read positions.
func (r *Room) PolicyRoomID() int64 {
	if r.ParentRoomID != nil {
		return *r.ParentRoomID
	}
	return r.ID
}

type ChatThread struct {
	Room          *Room
	MessageCount  int
	UnreadCount   int
	LastMessageAt *time.Time
}
