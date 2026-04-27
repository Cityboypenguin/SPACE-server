package model

const (
	RoomTypeCommunity = "community"
	RoomTypeDM        = "dm"
)

type Room struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type CreateRoomParam struct {
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type UpdateRoomParam struct {
	Name *string `json:"name,omitempty"`
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
