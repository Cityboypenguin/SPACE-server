package graph

import (
	"context"
	gql "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/upload"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/usecase/chatstamp"
	"strconv"
)

func (r *Resolver) graphStamp(v *model.ChatStamp) *gql.ChatStamp {
	url := ""
	if v.ImageKey != "" {
		url = r.StorageRepository.PublicURL(v.ImageKey)
	}
	return &gql.ChatStamp{ID: v.ID, Label: v.Label, ImageURL: url, DisplaySize: v.DisplaySize, Status: v.Status, AllowMessage: v.AllowMessage, AllowReaction: v.AllowReaction, MessageOrder: int32(v.MessageOrder), ReactionOrder: int32(v.ReactionOrder), Version: int32(v.Version)}
}
func (r *mutationResolver) AdminSaveChatStamp(ctx context.Context, input gql.ChatStampInput) (*gql.ChatStamp, error) {
	id := ""
	if input.ID != nil {
		id = *input.ID
	}
	v, e := r.StampService.Save(ctx, model.ChatStamp{ID: id, Label: input.Label, DisplaySize: input.DisplaySize, Status: input.Status, AllowMessage: input.AllowMessage, AllowReaction: input.AllowReaction, MessageOrder: int(input.MessageOrder), ReactionOrder: int(input.ReactionOrder), Version: int64(input.Version)}, input.UploadKey)
	if e != nil {
		return nil, e
	}
	return r.graphStamp(v), nil
}
func (r *mutationResolver) AdminReorderChatStamps(ctx context.Context, items []*gql.ChatStampOrderInput, reaction bool) (bool, error) {
	order := make([]chatstamp.OrderItem, 0, len(items))
	for _, v := range items {
		order = append(order, chatstamp.OrderItem{ID: v.ID, Version: int64(v.Version)})
	}
	e := r.StampService.Reorder(ctx, order, reaction)
	return e == nil, e
}
func (r *queryResolver) ChatStamps(ctx context.Context) ([]*gql.ChatStamp, error) {
	return r.listGraphStamps(ctx, false)
}
func (r *queryResolver) AdminChatStamps(ctx context.Context) ([]*gql.ChatStamp, error) {
	return r.listGraphStamps(ctx, true)
}
func (r *Resolver) listGraphStamps(ctx context.Context, admin bool) ([]*gql.ChatStamp, error) {
	rows, e := r.StampService.List(ctx, admin)
	if e != nil {
		return nil, e
	}
	out := make([]*gql.ChatStamp, 0, len(rows))
	for _, v := range rows {
		out = append(out, r.graphStamp(v))
	}
	return out, nil
}
func (r *queryResolver) AdminStampUploadURL(ctx context.Context, contentType string) (*gql.PresignedUploadURL, error) {
	actor, e := requireAdminAuth(ctx)
	if e != nil {
		return nil, e
	}
	return r.presignedUploadURL(ctx, upload.ChatStamp, strconv.FormatInt(actor.ID, 10), contentType)
}
