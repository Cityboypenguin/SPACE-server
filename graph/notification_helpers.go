package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

func (r *Resolver) buildPostMapFromNotifications(ctx context.Context, notifications []*model.Notification) map[int64]*model.Post {
	postMap := map[int64]*model.Post{}
	for _, n := range notifications {
		if n.TargetType == nil || *n.TargetType != notificationTargetTypePost || n.TargetID == nil {
			continue
		}
		if _, ok := postMap[*n.TargetID]; ok {
			continue
		}
		if p, err := r.GetPostByIDUseCase.Execute(ctx, *n.TargetID); err == nil && p != nil {
			postMap[*n.TargetID] = p
		}
	}
	return postMap
}

func (r *Resolver) buildPostMapFromNotificationGroups(ctx context.Context, groups []*model.NotificationGroup) map[int64]*model.Post {
	postMap := map[int64]*model.Post{}
	for _, g := range groups {
		if g.TargetType == nil || *g.TargetType != notificationTargetTypePost || g.TargetID == nil {
			continue
		}
		if _, ok := postMap[*g.TargetID]; ok {
			continue
		}
		if p, err := r.GetPostByIDUseCase.Execute(ctx, *g.TargetID); err == nil && p != nil {
			postMap[*g.TargetID] = p
		}
	}
	return postMap
}
