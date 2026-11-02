package graph

import (
	"context"
	"fmt"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
)

func (r *mutationResolver) CreateChatThread(ctx context.Context, parentRoomID string, title string) (*gqlmodel.ChatThread, error) {
	id, err := decodeGraphID(ctx, "room", parentRoomID)
	if err != nil {
		return nil, err
	}
	thread, err := r.ThreadService.Create(ctx, id, title)
	if err != nil {
		return nil, err
	}
	r.publishChatThreadChanged(thread.Room)
	return r.toGraphThread(ctx, thread)
}
func (r *mutationResolver) UpdateChatThread(ctx context.Context, roomID string, title *string, closed *bool) (*gqlmodel.ChatThread, error) {
	id, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, err
	}
	thread, err := r.ThreadService.Update(ctx, id, title, closed)
	if err != nil {
		return nil, err
	}
	r.publishChatThreadChanged(thread.Room)
	return r.toGraphThread(ctx, thread)
}
func (r *queryResolver) ChatThread(ctx context.Context, roomID string) (*gqlmodel.ChatThread, error) {
	id, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, err
	}
	thread, err := r.ThreadService.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.toGraphThread(ctx, thread)
}
func (r *queryResolver) ChatThreads(ctx context.Context, parentRoomID string, limit *int32, offset *int32) ([]*gqlmodel.ChatThread, error) {
	id, err := decodeGraphID(ctx, "room", parentRoomID)
	if err != nil {
		return nil, err
	}
	n, start := 20, 0
	if limit != nil {
		n = int(*limit)
	}
	if offset != nil {
		start = int(*offset)
	}
	threads, err := r.ThreadService.List(ctx, id, n, start)
	if err != nil {
		return nil, err
	}
	out := make([]*gqlmodel.ChatThread, 0, len(threads))
	for _, t := range threads {
		v, err := r.toGraphThread(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *queryResolver) HasUnreadChatThreads(ctx context.Context, parentRoomID string) (bool, error) {
	id, err := decodeGraphID(ctx, "room", parentRoomID)
	if err != nil {
		return false, err
	}
	return r.ThreadService.HasUnread(ctx, id)
}

func (r *subscriptionResolver) ChatThreadChanged(ctx context.Context, parentRoomID string) (<-chan string, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	pid, err := decodeGraphID(ctx, "room", parentRoomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	// スレッドは親ルームの権限に従うので、親ルームを読めれば購読できる。
	if _, err := r.requireRoomReadAccess(ctx, pid); err != nil {
		return nil, err
	}
	return subscribeTopicGuarded(
		ctx,
		r.PubSub,
		chatThreadChangedTopic(pid),
		subscriptionScope{UserID: claims.ID, RoomID: parentRoomID},
		r.roomReadGuard(pid),
		func(ev *ChatThreadChanged) (string, bool) { return ev.ThreadID, true },
	), nil
}
