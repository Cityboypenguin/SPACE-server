package model

type RoomUser struct {
	ID        int64  `json:"id"`
	RoomID    int64  `json:"room_id"`
	UserID    int64  `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type CreateRoomUserParam struct {
	RoomID    int64  `json:"room_id"`
	UserID    int64  `json:"user_id"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type UpdateRoomUserParam struct {
	Role *string `json:"role,omitempty"`
}