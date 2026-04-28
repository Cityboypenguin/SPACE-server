package model

import "time"

type Community struct {
	ID          int64  `json:"id"`
	RoomID      int64  `json:"room_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type CreateCommunityParam struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UpdateCommunityParam struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (c *Community) CreateCommunity(param CreateCommunityParam, roomID int64) {
	c.RoomID = roomID
	c.Name = param.Name
	c.Description = param.Description
	c.CreatedAt = param.CreatedAt.Unix()
	c.UpdatedAt = param.UpdatedAt.Unix()
}

func (c *Community) UpdateCommunity(param UpdateCommunityParam) {
	if param.Name != nil {
		c.Name = *param.Name
	}
	if param.Description != nil {
		c.Description = *param.Description
	}
	c.UpdatedAt = time.Now().Unix()
}
