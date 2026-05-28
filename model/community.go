package model

import "time"

type Community struct {
	ID          int64
	RoomID      int64
	Name        string
	Description string
	AvatarKey   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateCommunityParam struct {
	Name        string
	Description string
	AvatarKey   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpdateCommunityParam struct {
	Name        *string
	Description *string
	AvatarKey   *string
}

func (c *Community) CreateCommunity(param CreateCommunityParam, roomID int64) {
	c.RoomID = roomID
	c.Name = param.Name
	c.Description = param.Description
	c.AvatarKey = param.AvatarKey
	c.CreatedAt = param.CreatedAt
	c.UpdatedAt = param.UpdatedAt
}

func (c *Community) UpdateCommunity(param UpdateCommunityParam) {
	if param.Name != nil {
		c.Name = *param.Name
	}
	if param.Description != nil {
		c.Description = *param.Description
	}
	if param.AvatarKey != nil {
		c.AvatarKey = *param.AvatarKey
	}
	c.UpdatedAt = time.Now()
}
