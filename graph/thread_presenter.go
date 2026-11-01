package graph

import (
	"context"
	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
)

func (r *Resolver) toGraphThread(ctx context.Context, t *model.ChatThread) (*gqlmodel.ChatThread, error) {
	if t == nil {
		return nil, nil
	}
	room := t.Room
	manage, canWrite := false, false
	if fieldRequested(ctx, "canManage") {
		var err error
		manage, err = r.ThreadService.CanManage(ctx, room)
		if err != nil {
			return nil, err
		}
	}
	if fieldRequested(ctx, "canWrite") {
		_, err := r.ChatAccess.EnsureWriteAccess(ctx, room.ID)
		canWrite = err == nil
	}
	out := &gqlmodel.ChatThread{RoomID: encodeGraphID("room", room.ID), ParentRoomID: encodeGraphID("room", room.PolicyRoomID()), Title: room.Name,
		IsClosed: room.ThreadClosedAt != nil, CanManage: manage, CanWrite: canWrite, CreatedAt: room.CreatedAt.Format(timeFormat), UpdatedAt: room.UpdatedAt.Format(timeFormat), MessageCount: int32(t.MessageCount), UnreadCount: int32(t.UnreadCount)}
	if t.LastMessageAt != nil {
		v := t.LastMessageAt.Format(timeFormat)
		out.LastMessageAt = &v
	}
	return out, nil
}
