package model

type Room struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type CreateRoomParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type UpdateRoomParam struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (r *Room) CreateRoom(param CreateRoomParam) {
	r.Name = param.Name
	r.Type = "group"
	r.Description = param.Description
	r.CreatedAt = param.CreatedAt
	r.UpdatedAt = param.UpdatedAt
}

func (r *Room) UpdateRoom(param UpdateRoomParam) {
	if param.Name != nil {
		r.Name = *param.Name
	}
	if param.Description != nil {
		r.Description = *param.Description
	}
}