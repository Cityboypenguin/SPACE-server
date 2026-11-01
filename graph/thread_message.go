package graph

import (
	"context"
	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
)

func (r *messageResolver) Thread(ctx context.Context, obj *gqlmodel.Message) (*gqlmodel.ChatThread, error) {
	if obj.Thread == nil {
		return nil, nil
	}
	id, err := decodeGraphID(ctx, "room", obj.Thread.RoomID)
	if err != nil {
		return nil, err
	}
	thread, err := r.ThreadService.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return r.toGraphThread(ctx, thread)
}
