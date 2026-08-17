package favorite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

type CreateFavoriteUseCase interface {
	Execute(ctx context.Context, param model.CreateFavoriteParam) (*model.Favorite, error)
}

var _ CreateFavoriteUseCase = &CreateFavoriteInteractor{}

type CreateFavoriteInteractor struct {
	favoriteRepo          repository.FavoriteRepository
	postRepo              repository.PostRepository
	notificationPublisher notificationuc.NotificationPublisher
}

func NewCreateFavoriteUseCase(
	favoriteRepo repository.FavoriteRepository,
	postRepo repository.PostRepository,
	notificationPublisher notificationuc.NotificationPublisher,
) CreateFavoriteUseCase {
	return &CreateFavoriteInteractor{
		favoriteRepo:          favoriteRepo,
		postRepo:              postRepo,
		notificationPublisher: notificationPublisher,
	}
}

func (uc *CreateFavoriteInteractor) Execute(ctx context.Context, param model.CreateFavoriteParam) (*model.Favorite, error) {
	post, err := uc.postRepo.GetPostByID(ctx, param.PostID)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, fmt.Errorf("投稿が見つかりません")
	}
	if post.UserID == param.UserID {
		return nil, fmt.Errorf("ユーザーは自分の投稿をお気に入りにできません")
	}

	exist, err := uc.favoriteRepo.GetFavoriteByUserIDAndPostID(ctx, param.UserID, param.PostID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if exist != nil {
		return nil, fmt.Errorf("すでにお気に入りに登録されています")
	}

	favorite := model.CreateFavorite(param)

	id, err := uc.favoriteRepo.CreateFavorite(ctx, favorite)
	if err != nil {
		return nil, err
	}

	favorite.ID = id

	// 投稿者へ「いいね」通知を送る。自分の投稿への いいね は上でリジェクト済みなので、
	// ここに到達した時点で通知相手は必ず別ユーザー。配信失敗は致命的ではないのでログのみ。
	if uc.notificationPublisher != nil {
		targetType := notificationuc.TargetPost
		if err := uc.notificationPublisher.Publish(ctx, notificationuc.PublishParams{
			UserID:     post.UserID,
			Type:       notificationuc.TypeFavorite,
			ActorID:    &param.UserID,
			TargetType: &targetType,
			TargetID:   &param.PostID,
			Message:    "あなたの投稿がいいねされました",
		}); err != nil {
			logger.Log.Error().Err(err).Msg("failed to publish favorite notification")
		}
	}

	return favorite, nil
}
