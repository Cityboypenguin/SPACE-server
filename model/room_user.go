package model

type RoomUser struct {
	ID        int64
	RoomID    int64
	UserID    int64
	Role      string
	CreatedAt int64
	UpdatedAt int64
}

type CreateRoomUserParam struct {
	RoomID    int64
	UserID    int64
	Role      string
	CreatedAt int64
	UpdatedAt int64
}

type UpdateRoomUserParam struct {
	Role *string
}

func (ru *RoomUser) CreateRoomUser(param CreateRoomUserParam) {
	ru.RoomID = param.RoomID
	ru.UserID = param.UserID
	ru.Role = param.Role
	ru.CreatedAt = param.CreatedAt
	ru.UpdatedAt = param.UpdatedAt
}

func (ru *RoomUser) UpdateRoomUser(param UpdateRoomUserParam) {
	if param.Role != nil {
		ru.Role = *param.Role
	}
}
