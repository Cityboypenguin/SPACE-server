package chat

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// SaveCommunityJoinNotice is called inside the transaction that creates a membership.
func (w MessageWriters) SaveCommunityJoinNotice(ctx context.Context, roomID, memberID int64) (*model.Message, error) {
	msg, err := model.NewMessage(model.CreateMessageParam{
		RoomID: roomID, UserID: memberID, Content: "メンバーが参加しました",
		AuthorRole: model.AuthorRoleCommunityWelcome,
	})
	if err != nil {
		return nil, err
	}
	return msg, w.system.SaveMessage(ctx, msg)
}

// SaveCommunityGreeting saves a regular stamp sent by a community member.
func (w MessageWriters) SaveCommunityGreeting(ctx context.Context, roomID, actorID int64, stampID string) (*model.Message, error) {
	msg, err := model.NewMessage(model.CreateMessageParam{RoomID: roomID, UserID: actorID, StampID: &stampID})
	if err != nil {
		return nil, err
	}
	return msg, w.system.SaveMessage(ctx, msg)
}
