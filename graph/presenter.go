package graph

import (
	"fmt"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
)

const timeFormat = "2006-01-02T15:04:05Z07:00"

func toGraphUser(user *model.User) *gqlmodel.User {
	if user == nil {
		return nil
	}
	return &gqlmodel.User{
		ID:        encodeGraphID("user", user.ID),
		AccountID: user.AccountID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Format(timeFormat),
		UpdatedAt: user.UpdatedAt.Format(timeFormat),
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
		CreatedAt: admin.CreatedAt.Format(timeFormat),
		UpdatedAt: admin.UpdatedAt.Format(timeFormat),
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
		CreatedAt: room.CreatedAt.Format(timeFormat),
		UpdatedAt: room.UpdatedAt.Format(timeFormat),
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
		CreatedAt:   c.CreatedAt.Format(timeFormat),
		UpdatedAt:   c.UpdatedAt.Format(timeFormat),
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
		CreatedAt: msg.CreatedAt.Format(timeFormat),
		UpdatedAt: msg.UpdatedAt.Format(timeFormat),
	}
}

func toGraphPost(post *model.Post, parent *gqlmodel.Post) *gqlmodel.Post {
	if post == nil {
		return nil
	}

	return &gqlmodel.Post{
		ID:        encodeGraphID("post", post.ID),
		User:      toGraphUser(post.User),
		Content:   post.Content,
		Parent:    parent,
		CreatedAt: post.CreatedAt.Format(timeFormat),
		UpdatedAt: post.UpdatedAt.Format(timeFormat),
	}
}
func toGraphProfile(user *model.User, profile *model.Profile) *gqlmodel.Profile {
	if user == nil {
		return nil
	}
	if profile == nil {
		return &gqlmodel.Profile{
			UserID:    encodeGraphID("user", user.ID),
			User:      toGraphUser(user),
			Username:  user.Name,
			CreatedAt: "0",
			UpdatedAt: "0",
		}
	}
	return &gqlmodel.Profile{
		UserID:    encodeGraphID("user", profile.UserID),
		User:      toGraphUser(user),
		Username:  user.Name,
		Bio:       &profile.Bio,
		Image:     &profile.Image,
		CreatedAt: fmt.Sprintf("%d", profile.CreatedAt),
		UpdatedAt: fmt.Sprintf("%d", profile.UpdatedAt),
	}
}
