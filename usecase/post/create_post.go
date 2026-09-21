package post

import (
	"context"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"

	"fmt"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

type CreatePostUseCase interface {
	Execute(ctx context.Context, param model.CreatePostParam, mediaInputs []model.MediaInput) (*model.Post, error)
}

var _ CreatePostUseCase = &CreatePostInteractor{}

type CreatePostInteractor struct {
	uploads               uploadusecase.Acceptor
	postRepo              postWriteRepository
	mediaRepo             postMediaRepository
	userRepo              repository.UserReader
	blockerRepo           repository.BlockerRepository
	txManager             repository.TxManager
	notificationPublisher notificationuc.NotificationPublisher
}

func NewCreatePostUseCase(uploads uploadusecase.Acceptor,
	postRepo postWriteRepository,
	mediaRepo postMediaRepository,
	userRepo repository.UserReader,
	blockerRepo repository.BlockerRepository,
	txManager repository.TxManager,
	notificationPublisher notificationuc.NotificationPublisher,
) CreatePostUseCase {
	return &CreatePostInteractor{
		uploads:               uploads,
		postRepo:              postRepo,
		mediaRepo:             mediaRepo,
		userRepo:              userRepo,
		blockerRepo:           blockerRepo,
		txManager:             txManager,
		notificationPublisher: notificationPublisher,
	}
}

func (uc *CreatePostInteractor) Execute(ctx context.Context, param model.CreatePostParam, mediaInputs []model.MediaInput) (_ *model.Post, err error) {
	// 投稿者は呼び出し元が渡す値ではなく ctx から決める。param.UserID に何が
	// 入っていても、ここで上書きされる（他人名義の投稿を作りようがない）。
	callerID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	param.UserID = callerID

	if strings.TrimSpace(param.Content) == "" && len(mediaInputs) == 0 {
		return nil, fmt.Errorf("content cannot be empty")
	}
	if err := validatePostContent(param.Content); err != nil {
		return nil, err
	}

	// 添付の受け入れはここで通す。リゾルバの手順にしておくと、このユースケースを
	// 直接呼ぶ経路が増えたときに検査を飛ばせてしまう。
	//
	// 種別と所有者の確認は受け入れの中で、ストレージに触る前に済む
	// （internal/upload.CheckKey）。保存が成立しなければ、公開した実体は取り消す。
	uploads := uploadusecase.Begin(uc.uploads)
	defer uploads.DiscardOnError(ctx, &err)

	mediaInputs, err = uploadusecase.AcceptAll(ctx, uploads, uploadusecase.Attachment, mediaInputs, func(m *model.MediaInput) *string { return &m.StorageKey })
	if err != nil {
		return nil, err
	}

	// メンション解決は参照のみなのでトランザクションの外で済ませる。
	// 失敗した場合はメンション無しとして投稿を続行する（本文はそのまま残る）。
	mentions, err := ResolveMentions(ctx, uc.userRepo, uc.blockerRepo, param.Content, param.UserID)
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to resolve mentions")
		mentions = nil
	}

	post := model.CreatePost(param)

	now := time.Now()
	post.CreatedAt = now
	post.UpdatedAt = now

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		id, err := uc.postRepo.CreatePost(ctx, post)
		if err != nil {
			return err
		}
		post.ID = id

		if tags := ExtractHashtags(post.Content); len(tags) > 0 {
			if err := uc.postRepo.CreatePostHashtags(ctx, post.ID, tags); err != nil {
				return err
			}
		}

		if len(mentions) > 0 {
			if err := uc.postRepo.CreatePostMentions(ctx, post.ID, mentions); err != nil {
				return err
			}
		}

		// 添付はまとめて保存する（media 行を1本、紐付けを1本）。以前は入力1件ごとに
		// 2往復していたので、4枚付けると8往復していた。
		if medias := model.NewMediaBatch(param.UserID, mediaInputs, now); len(medias) > 0 {
			if err := uc.mediaRepo.CreateMediaBatch(ctx, medias); err != nil {
				return err
			}
			if err := uc.mediaRepo.CreatePostMediaBatch(ctx, post.ID, model.MediaIDs(medias), 0); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// 返信の場合、親投稿の投稿者へ通知する（自分自身への返信は除く）。
	// 配信失敗は投稿作成の成否に影響させない。
	// repliedTo には返信通知を送った相手を控えておき、同じ人を本文でメンションしていても
	// 通知が二重にならないようにする。
	var repliedTo *int64
	if uc.notificationPublisher != nil && param.ParentID != nil {
		parent, perr := uc.postRepo.GetPostByID(ctx, *param.ParentID)
		if perr != nil {
			// 親投稿が引けないと通知先が決まらないので諦める（投稿自体は成立済み）。
			// 削除済みで nil が返るのは正常系なので、error のときだけ残す。
			logger.Log.Error().Err(perr).
				Str("component", "post").
				Int64("post_id", post.ID).
				Int64("parent_post_id", *param.ParentID).
				Msg("failed to load the parent post; skipping the reply notification")
		}
		if perr == nil && parent != nil && parent.UserID != param.UserID {
			targetType := notificationuc.TargetPost
			if err := uc.notificationPublisher.Publish(ctx, notificationuc.PublishParams{
				UserID:     parent.UserID,
				Type:       notificationuc.TypeReply,
				ActorID:    &param.UserID,
				TargetType: &targetType,
				TargetID:   param.ParentID,
				Message:    notificationuc.MessageRepliedToPost,
			}); err != nil {
				logger.Log.Error().Err(err).
					Str("component", "post").
					Int64("post_id", post.ID).
					Int64("recipient_id", parent.UserID).
					Msg("failed to publish reply notification")
			} else {
				repliedTo = &parent.UserID
			}
		}
	}

	NotifyMentions(ctx, uc.notificationPublisher, mentions, post.ID, param.UserID, repliedTo)

	return post, nil
}
