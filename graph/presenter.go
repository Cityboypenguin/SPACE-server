package graph

import (
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

func toGraphCommunity(c *model.Community, avatarURL string) *gqlmodel.Community {
	if c == nil {
		return nil
	}

	return &gqlmodel.Community{
		ID:          encodeGraphID("community", c.ID),
		RoomID:      encodeGraphID("room", c.RoomID),
		Name:        c.Name,
		Description: c.Description,
		AvatarURL:   avatarURL,
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

func toGraphMedia(m *model.Media, url string) *gqlmodel.Media {
	if m == nil {
		return nil
	}
	return &gqlmodel.Media{
		ID:          encodeGraphID("media", m.ID),
		URL:         url,
		ContentType: m.ContentType,
		CreatedAt:   m.CreatedAt.Format(timeFormat),
	}
}

func toGraphPost(post *model.Post) *gqlmodel.Post {
	if post == nil {
		return nil
	}

	var gqlParent *gqlmodel.Post
	if post.ParentID != nil {
		gqlParent = &gqlmodel.Post{ID: encodeGraphID("post", *post.ParentID)}
	}

	var deletedAt *string
	if post.DeletedAt != nil {
		formatted := post.DeletedAt.Format(timeFormat)
		deletedAt = &formatted
	}

	return &gqlmodel.Post{
		ID:         encodeGraphID("post", post.ID),
		Content:    post.Content,
		CreatedAt:  post.CreatedAt.Format(timeFormat),
		UpdatedAt:  post.UpdatedAt.Format(timeFormat),
		DeletedAt:  deletedAt,
		User:       toGraphUser(&model.User{ID: post.UserID}),
		Parent:     gqlParent,
		ReplyCount: int32(post.ReplyCount),
	}
}
func toGraphProfile(user *model.User, profile *model.Profile, avatarURL *string) *gqlmodel.Profile {
	if user == nil {
		return nil
	}
	if profile == nil {
		return &gqlmodel.Profile{
			User:      toGraphUser(user),
			Username:  user.Name,
			CreatedAt: "0",
			UpdatedAt: "0",
		}
	}
	return &gqlmodel.Profile{
		User:      toGraphUser(user),
		Username:  user.Name,
		Bio:       &profile.Bio,
		AvatarURL: avatarURL,
		CreatedAt: profile.CreatedAt.Format(timeFormat),
		UpdatedAt: profile.UpdatedAt.Format(timeFormat),
	}
}
func toGraphFavoriteUser(fu *model.FavoriteUser) *gqlmodel.FavoriteUser {
	if fu == nil {
		return nil
	}
	return &gqlmodel.FavoriteUser{
		ID:             encodeGraphID("favorite_user", fu.ID),
		UserID:         encodeGraphID("user", fu.UserID),
		FavoriteUserID: encodeGraphID("user", fu.FavoriteUserID),
		CreatedAt:      fu.CreatedAt.Format(timeFormat),
	}
}

func toGraphBlocker(b *model.Blocker) *gqlmodel.Blocker {
	if b == nil {
		return nil
	}
	return &gqlmodel.Blocker{
		ID:            encodeGraphID("blocker", b.ID),
		UserID:        encodeGraphID("user", b.UserID),
		BlockedUserID: encodeGraphID("user", b.BlockedUserID),
		CreatedAt:     b.CreatedAt.Format(timeFormat),
	}
}
