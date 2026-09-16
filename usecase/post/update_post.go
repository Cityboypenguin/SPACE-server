package post

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

type UpdatePostUseCase interface {
	Execute(ctx context.Context, param model.UpdatePostParam, newMediaInputs []model.MediaInput, deletedMediaIDs []int64) (*model.Post, error)
}

var _ UpdatePostUseCase = &UpdatePostInteractor{}

type UpdatePostInteractor struct {
	postRepo              repository.PostRepository
	mediaRepo             repository.MediaRepository
	userRepo              repository.UserRepository
	blockerRepo           repository.BlockerRepository
	txManager             repository.TxManager
	notificationPublisher notificationuc.NotificationPublisher
}

func NewUpdatePostUseCase(
	postRepo repository.PostRepository,
	mediaRepo repository.MediaRepository,
	userRepo repository.UserRepository,
	blockerRepo repository.BlockerRepository,
	txManager repository.TxManager,
	notificationPublisher notificationuc.NotificationPublisher,
) *UpdatePostInteractor {
	return &UpdatePostInteractor{
		postRepo:              postRepo,
		mediaRepo:             mediaRepo,
		userRepo:              userRepo,
		blockerRepo:           blockerRepo,
		txManager:             txManager,
		notificationPublisher: notificationPublisher,
	}
}

func (uc *UpdatePostInteractor) Execute(ctx context.Context, param model.UpdatePostParam, newMediaInputs []model.MediaInput, deletedMediaIDs []int64) (*model.Post, error) {
	if param.Content != nil {
		if err := validatePostContent(*param.Content); err != nil {
			return nil, err
		}
	}

	prefix := fmt.Sprintf("media/%d/", param.UserID)
	for _, input := range newMediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
	}

	post, err := uc.postRepo.GetPostByID(ctx, param.PostID)
	if err != nil {
		return nil, err
	}
	if post == nil || post.UserID != param.UserID {
		return nil, fmt.Errorf("post not found or unauthorized")
	}

	// 本文が変わるときだけメンションを解決し直す。
	// 既に通知済みの相手には再通知しないよう、編集前のメンションを控えておく。
	var newMentions []*model.Mention
	alreadyNotified := make(map[int64]struct{})
	if param.Content != nil {
		before, err := uc.postRepo.ListMentionsByPostIDs(ctx, []int64{post.ID})
		if err != nil {
			return nil, err
		}
		for _, m := range before[post.ID] {
			alreadyNotified[m.UserID] = struct{}{}
		}

		newMentions, err = ResolveMentions(ctx, uc.userRepo, uc.blockerRepo, *param.Content, param.UserID)
		if err != nil {
			logger.Log.Error().Err(err).Msg("failed to resolve mentions")
			newMentions = nil
		}
	}

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {

		post.UpdatePost(param)

		if err := uc.postRepo.UpdatePost(ctx, post); err != nil {
			return err
		}

		// 本文が更新された場合はハッシュタグを再同期する。
		if param.Content != nil {
			if err := uc.postRepo.DeletePostHashtagsByPostID(ctx, post.ID); err != nil {
				return err
			}
			if tags := ExtractHashtags(post.Content); len(tags) > 0 {
				if err := uc.postRepo.CreatePostHashtags(ctx, post.ID, tags); err != nil {
					return err
				}
			}

			// メンションも同様に貼り直す。本文から消えたメンションは行ごと消えるため、
			// 表示側のリンクも自動的に消える。
			if err := uc.postRepo.DeletePostMentionsByPostID(ctx, post.ID); err != nil {
				return err
			}
			if len(newMentions) > 0 {
				if err := uc.postRepo.CreatePostMentions(ctx, post.ID, newMentions); err != nil {
					return err
				}
			}
		}

		for _, mediaID := range deletedMediaIDs {
			if err := uc.mediaRepo.DeleteMediaByIDAndUserID(ctx, mediaID, param.UserID); err != nil {
				return err
			}
		}

		if len(newMediaInputs) > 0 {
			currentMaxPos, err := uc.mediaRepo.GetMaxPostMediaPosition(ctx, param.PostID)
			if err != nil {
				return err
			}

			for i, input := range newMediaInputs {
				media := model.NewMedia(param.UserID, input, post.UpdatedAt)

				if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
					return err
				}

				newPosition := currentMaxPos + 1 + i

				if err := uc.mediaRepo.CreatePostMedia(ctx, post.ID, media.ID, newPosition); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// 編集で新しく追加されたメンションだけ通知する（既にメンション済みの相手は再通知しない）。
	added := make([]*model.Mention, 0, len(newMentions))
	for _, m := range newMentions {
		if _, ok := alreadyNotified[m.UserID]; ok {
			continue
		}
		added = append(added, m)
	}
	NotifyMentions(ctx, uc.notificationPublisher, added, post.ID, param.UserID, nil)

	return post, nil
}
