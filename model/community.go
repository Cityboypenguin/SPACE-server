package model

import "time"

type Community struct {
	ID          int64
	RoomID      int64
	Name        string
	Description string
	AvatarMedia *Media
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpdateCommunityParam struct {
	Name        *string
	Description *string
}

func (c *Community) UpdateCommunity(param UpdateCommunityParam) {
	if param.Name != nil {
		c.Name = *param.Name
	}
	if param.Description != nil {
		c.Description = *param.Description
	}
	c.UpdatedAt = time.Now()
}
