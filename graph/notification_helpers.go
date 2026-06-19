package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

func (r *Resolver) buildPostMapFromNotifications(ctx context.Context, notifications []*model.Notification) (map[int64]*model.Post, error) {
	seen := map[int64]struct{}{}
	var postIDs []int64
	for _, n := range notifications {
		if n.TargetType == nil || *n.TargetType != notificationTargetTypePost || n.TargetID == nil {
			continue
		}
		if _, ok := seen[*n.TargetID]; ok {
			continue
		}
		seen[*n.TargetID] = struct{}{}
		postIDs = append(postIDs, *n.TargetID)
	}
	return r.buildPostMapByIDs(ctx, postIDs)
}

func (r *Resolver) buildPostMapFromNotificationGroups(ctx context.Context, groups []*model.NotificationGroup) (map[int64]*model.Post, error) {
	seen := map[int64]struct{}{}
	var postIDs []int64
	for _, g := range groups {
		if g.TargetType == nil || *g.TargetType != notificationTargetTypePost || g.TargetID == nil {
			continue
		}
		if _, ok := seen[*g.TargetID]; ok {
			continue
		}
		seen[*g.TargetID] = struct{}{}
		postIDs = append(postIDs, *g.TargetID)
	}
	return r.buildPostMapByIDs(ctx, postIDs)
}

func (r *Resolver) buildPostMapByIDs(ctx context.Context, postIDs []int64) (map[int64]*model.Post, error) {
	postMap := map[int64]*model.Post{}
	if len(postIDs) == 0 {
		return postMap, nil
	}
	posts, err := r.GetPostsByIDsUseCase.Execute(ctx, postIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range posts {
		postMap[p.ID] = p
	}
	return postMap, nil
}
