package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListMentionsByPostIDsUseCase は投稿IDごとのメンション一覧をまとめて引く。
// 投稿一覧（タイムライン）で1件ずつ引くと N+1 になるため DataLoader から使う。
type ListMentionsByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Mention, error)
}

var _ ListMentionsByPostIDsUseCase = &ListMentionsByPostIDsInteractor{}

type ListMentionsByPostIDsInteractor struct {
	postRepo repository.PostRepository
}

func NewListMentionsByPostIDsUseCase(postRepo repository.PostRepository) ListMentionsByPostIDsUseCase {
	return &ListMentionsByPostIDsInteractor{postRepo: postRepo}
}

func (uc *ListMentionsByPostIDsInteractor) Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Mention, error) {
	return uc.postRepo.ListMentionsByPostIDs(ctx, postIDs)
}
