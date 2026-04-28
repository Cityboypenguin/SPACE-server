package graph

import (
	"fmt"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
)

func toGraphUser(user *model.User) *gqlmodel.User {
	if user == nil {
		return nil
	}
	return &gqlmodel.User{
		ID:        encodeGraphID("user", user.ID),
		UserID:    user.UserID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: fmt.Sprintf("%d", user.CreatedAt),
		UpdatedAt: fmt.Sprintf("%d", user.UpdatedAt),
	}
}

func toGraphAdministrator(admin *model.Administrator) *gqlmodel.Administrator {
	if admin == nil {
		return nil
	}
	return &gqlmodel.Administrator{
		ID:        encodeGraphID("administrator", admin.ID),
		Name:      admin.Name,
		Email:     admin.Email,
		CreatedAt: fmt.Sprintf("%d", admin.CreatedAt),
		UpdatedAt: fmt.Sprintf("%d", admin.UpdatedAt),
	}
}

func toGraphRoom(room *model.Room) *gqlmodel.Room {
	if room == nil {
		return nil
	}
	return &gqlmodel.Room{
		ID:        encodeGraphID("room", room.ID),
		Name:      room.Name,
		Type:      room.Type,
		CreatedAt: fmt.Sprintf("%d", room.CreatedAt),
		UpdatedAt: fmt.Sprintf("%d", room.UpdatedAt),
	}
}

func toGraphCommunity(c *model.Community) *gqlmodel.Community {
	if c == nil {
		return nil
	}
	return &gqlmodel.Community{
		ID:          encodeGraphID("community", c.ID),
		RoomID:      encodeGraphID("room", c.RoomID),
		Name:        c.Name,
		Description: c.Description,
		CreatedAt:   fmt.Sprintf("%d", c.CreatedAt),
		UpdatedAt:   fmt.Sprintf("%d", c.UpdatedAt),
	}
}

func toGraphMessage(msg *model.Message) *gqlmodel.Message {
	if msg == nil {
		return nil
	}
	return &gqlmodel.Message{
		ID:        encodeGraphID("message", msg.ID),
		RoomID:    encodeGraphID("room", msg.RoomID),
		UserID:    encodeGraphID("user", msg.UserID),
		Content:   msg.Content,
		CreatedAt: fmt.Sprintf("%d", msg.CreatedAt),
		UpdatedAt: fmt.Sprintf("%d", msg.UpdatedAt),
	}
}
