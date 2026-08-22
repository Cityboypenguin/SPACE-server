package model

import "time"

const (
	RoomTypeCommunity = "community"
	RoomTypeDM        = "dm"
	RoomTypeCourse    = "course"
)

type Room struct {
	ID        int64
	Name      string
	Type      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateRoomParam struct {
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdateRoomParam struct {
	Name *string
}

func (r *Room) CreateRoom(param CreateRoomParam) {
	r.Name = param.Name
	r.Type = RoomTypeCommunity
	r.CreatedAt = param.CreatedAt
	r.UpdatedAt = param.UpdatedAt
}

func (r *Room) UpdateRoom(param UpdateRoomParam) {
	if param.Name != nil {
		r.Name = *param.Name
	}
}
