package chat

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// SaveCommunityWelcome is called inside the transaction that creates a membership.
func (w MessageWriters) SaveCommunityWelcome(ctx context.Context, roomID, memberID int64, stampID *string) (*model.Message, error) {
	msg, err := model.NewMessage(model.CreateMessageParam{
		RoomID: roomID, UserID: memberID, Content: "メンバーが参加しました",
		AuthorRole: model.AuthorRoleCommunityWelcome, StampID: stampID,
	})
	if err != nil {
		return nil, err
	}
	return msg, w.system.SaveMessage(ctx, msg)
}

// SaveCommunityGreeting is called after the membership and selected stamp have been checked.
func (w MessageWriters) SaveCommunityGreeting(ctx context.Context, roomID, actorID int64, stampID string) (*model.Message, error) {
	msg, err := model.NewMessage(model.CreateMessageParam{RoomID: roomID, UserID: actorID, StampID: &stampID})
	if err != nil {
		return nil, err
	}
	return msg, w.system.SaveMessage(ctx, msg)
}
