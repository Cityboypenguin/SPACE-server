// Package courseimage は授業内チャットに上がった画像の一元管理（ADM-05）。
// メッセージ・質問・回答の画像を管理画面でまとめて一覧し、削除できる。
// 理由: 授業資料の共有が横行するのを防ぐため。
package courseimage

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/upload"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
	"github.com/Cityboypenguin/SPACE-server/usecase/reportedmedia"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// Deps は Service の依存。配信まわり（Messages 以降）は開いている画面から画像を消すために使う。
type Deps struct {
	Images repository.CourseChatImageRepository
	// Reports は通報時点の写しが指している添付を付け替えるために使う。
	Reports repository.ReportedMediaRepository
	// Public は質問・回答の添付、MessageStore はメッセージの添付の置き場（非公開）。
	// 外した画像も MessageStore へ移す。
	Public       repository.StorageRepository
	MessageStore repository.StorageRepository

	Messages       MessageReader
	Questions      QuestionReader
	Answers        AnswerReader
	GetRoom        roomusecase.GetRoomUseCase
	ChatEvents     MessageUpdatePublisher
	QuestionEvents questionusecase.EventPublisher
	AnswerEvents   answerusecase.EventPublisher
}

// MessageReader / QuestionReader / AnswerReader は配信する親を引く口。
type MessageReader interface {
	GetMessageByID(ctx context.Context, id int64) (*model.Message, error)
}

type QuestionReader interface {
	GetQuestionByID(ctx context.Context, id int64) (*model.Question, error)
}

type AnswerReader interface {
	GetAnswerByID(ctx context.Context, id int64) (*model.Answer, error)
}

// MessageUpdatePublisher はチャットの配信のうち、ここで使うぶん。
type MessageUpdatePublisher interface {
	MessageUpdated(ctx context.Context, ev chatusecase.MessageUpdatedEvent)
}

// RemovedRetention は外した画像のうち、通報のないものを残しておく期間。
const RemovedRetention = 30 * 24 * time.Hour

type Service struct {
	d   Deps
	now func() time.Time
}

func NewService(d Deps) *Service {
	return &Service{d: d, now: time.Now}
}

// storeFor は画像の実体がある置き場を返す。メッセージの添付は非公開の置き場にあり、分離前の
// 添付だけが公開側に残っている（graph の messageMediaURL と同じ見分け方）。
func (s *Service) storeFor(source model.CourseChatImageSource, key string) repository.StorageRepository {
	if source != model.CourseChatImageSourceMessage {
		return s.d.Public
	}
	if kind, ok := upload.KindForObjectKey(key); ok && !kind.Private {
		return s.d.Public
	}
	return s.d.MessageStore
}

func (s *Service) List(ctx context.Context, filter repository.CourseChatImageFilter, limit, offset int) ([]*model.CourseChatImage, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return s.d.Images.ListCourseChatImages(ctx, filter, limit, offset)
}

// Delete は授業内チャットの画像を利用者の目から外す。
//
// ファイルは消さず、利用者の届かない非公開の置き場へ移す。公開の置き場に残すと元のURLで開けて
// しまうので、その場に残すだけにはしない。通報の写しは移した先を指すように付け替えるので、
// 通報の詳細では見られる。通報のない画像は RemovedRetention のあと PurgeExpired が消す。
//
// 手順は移す→付け替え→元のファイルを消す→記録を移す。途中で失敗したらエラーで返し、やり直せる
// （記録を先に移すと、元のファイルが公開されたまま残る）。
// 授業内チャットの画像でないメディア（アイコンなど）は、この経路では消せない。
func (s *Service) Delete(ctx context.Context, mediaID int64) error {
	claims, err := authz.RequireAdmin(ctx)
	if err != nil {
		return err
	}
	img, err := s.d.Images.GetCourseChatImage(ctx, mediaID)
	if err != nil {
		return err
	}
	if img == nil {
		return apperr.NotFound("画像が見つかりません")
	}
	removedKey := upload.RemovedKeyFor(mediaID, img.Media.StorageKey)
	if err := s.moveToRemoved(ctx, img, removedKey); err != nil {
		logger.Log.Error().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to move the image out of reach")
		return err
	}
	if err := s.storeFor(img.Source, img.Media.StorageKey).DeleteObject(ctx, img.Media.StorageKey); err != nil {
		logger.Log.Error().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to delete the stored object")
		return err
	}
	if err := s.d.Images.RemoveCourseChatImage(ctx, &model.RemovedCourseChatImage{
		MediaID: mediaID, Source: img.Source, TargetID: img.TargetID, Position: img.Position,
		RoomID: img.RoomID, CourseName: img.CourseName, Uploader: img.Uploader,
		RemovedKey: removedKey, OriginalKey: img.Media.StorageKey, ContentType: img.Media.ContentType,
		Width: img.Media.Width, Height: img.Media.Height, CreatedAt: img.Media.CreatedAt,
		RemovedAt: s.now(), RemovedBy: claims.ID,
	}); err != nil {
		return err
	}
	audit.LogCourseChatImageDeleted(ctx, mediaID, string(img.Source), img.TargetID, img.Uploader.ID, removedKey)

	// 移している間に作られた通報の写しは元のキーを指しているので、記録を移したあとで付け替え直す。
	// ここから先の失敗で削除を失敗にはしない（画像はもう利用者から見えない）。
	targetType, targetID := reportTarget(img.Source, img.TargetID)
	if err := s.d.Reports.RepointReportedMedia(ctx, targetType, targetID, img.Media.StorageKey, removedKey); err != nil {
		logger.Log.Warn().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to repoint reports created during the deletion")
	}
	s.broadcast(ctx, img.Source, img.TargetID)
	return nil
}

// moveToRemoved は画像を非公開の置き場へ写し、通報の写しをそちらへ付け替える。
// 実体がもう無ければ（元のファイルを消したあとのやり直しなど）写さない。
func (s *Service) moveToRemoved(ctx context.Context, img *model.CourseChatImage, removedKey string) error {
	key := img.Media.StorageKey
	err := reportedmedia.CopyObject(ctx, s.storeFor(img.Source, key), key, s.d.MessageStore, removedKey, img.Media.ContentType)
	if errors.Is(err, repository.ErrObjectNotFound) {
		logger.Log.Warn().Str("component", "course_chat_image").Int64("media_id", img.Media.ID).Msg("the image has no stored object")
		return nil
	}
	if err != nil {
		return err
	}
	targetType, targetID := reportTarget(img.Source, img.TargetID)
	return s.d.Reports.RepointReportedMedia(ctx, targetType, targetID, key, removedKey)
}

// ListRemoved は外した画像の一覧。通報の写しが指しているかも入れる。
func (s *Service) ListRemoved(ctx context.Context, filter repository.CourseChatImageFilter, limit, offset int) ([]*model.RemovedCourseChatImage, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	items, total, err := s.d.Images.ListRemovedCourseChatImages(ctx, filter, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for _, img := range items {
		if img.Reported, err = s.isReported(ctx, img); err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

func (s *Service) isReported(ctx context.Context, img *model.RemovedCourseChatImage) (bool, error) {
	targetType, targetID := reportTarget(img.Source, img.TargetID)
	return s.d.Reports.HasReportedMedia(ctx, targetType, targetID, img.RemovedKey)
}

// Restore は外した画像を元の投稿へ戻す。投稿か投稿者がもう無ければ戻せない。
//
// 手順はファイルを戻す→記録を戻す→通報の写しを付け替える→残したファイルを消す。記録を戻せな
// ければ戻したファイルを消す。写しを付け替えられなければ、残したファイルは写しのために消さない。
func (s *Service) Restore(ctx context.Context, mediaID int64) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	img, err := s.d.Images.GetRemovedCourseChatImage(ctx, mediaID)
	if err != nil {
		return err
	}
	if img == nil {
		return apperr.NotFound("画像が見つかりません")
	}
	if err := s.d.Images.CheckRestorable(ctx, img); err != nil {
		return translateRestoreError(err)
	}
	store := s.storeFor(img.Source, img.OriginalKey)
	if err := reportedmedia.CopyObject(ctx, s.d.MessageStore, img.RemovedKey, store, img.OriginalKey, img.ContentType); err != nil {
		if errors.Is(err, repository.ErrObjectNotFound) {
			return apperr.NotFound("画像のファイルが見つかりません")
		}
		return err
	}
	if err := s.d.Images.RestoreCourseChatImage(ctx, img); err != nil {
		if delErr := store.DeleteObject(ctx, img.OriginalKey); delErr != nil {
			logger.Log.Warn().Err(delErr).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to clean up the restored copy")
		}
		return translateRestoreError(err)
	}
	audit.LogCourseChatImageRestored(ctx, mediaID, string(img.Source), img.TargetID)

	targetType, targetID := reportTarget(img.Source, img.TargetID)
	if err := s.d.Reports.RepointReportedMedia(ctx, targetType, targetID, img.RemovedKey, img.OriginalKey); err != nil {
		logger.Log.Warn().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to repoint reports; keeping the removed copy")
	} else if err := s.d.MessageStore.DeleteObject(ctx, img.RemovedKey); err != nil {
		logger.Log.Warn().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to delete the removed copy")
	}
	s.broadcast(ctx, img.Source, img.TargetID)
	return nil
}

func translateRestoreError(err error) error {
	switch {
	case errors.Is(err, repository.ErrRestoreTargetGone):
		return apperr.Conflict("投稿が削除されているため、元に戻せません")
	case errors.Is(err, repository.ErrRestoreUploaderGone):
		return apperr.Conflict("投稿者が退会しているため、元に戻せません")
	}
	return err
}

// purgeBatchSize は期限切れの画像を1度に読む件数。
const purgeBatchSize = 100

// PurgeExpired は保持期間を過ぎた、通報のない画像を完全に消す。消した件数を返す。
// 通報の写しが指している画像は、通報が残る間は残す。
// 1件の失敗で止めず、残りを処理してからエラーを返す（次の実行でやり直す）。
func (s *Service) PurgeExpired(ctx context.Context, now time.Time) (int, error) {
	before := now.Add(-RemovedRetention)
	purged := 0
	var firstErr error
	var after int64
	for {
		items, err := s.d.Images.ListRemovedCourseChatImagesBefore(ctx, before, after, purgeBatchSize)
		if err != nil {
			return purged, err
		}
		for _, img := range items {
			after = img.MediaID
			done, err := s.purgeOne(ctx, img)
			if err != nil {
				logger.Log.Error().Err(err).Str("component", "course_chat_image").Int64("media_id", img.MediaID).Msg("failed to purge the removed image")
				if firstErr == nil {
					firstErr = err
				}
			} else if done {
				purged++
			}
		}
		if len(items) < purgeBatchSize {
			return purged, firstErr
		}
	}
}

func (s *Service) purgeOne(ctx context.Context, img *model.RemovedCourseChatImage) (bool, error) {
	reported, err := s.isReported(ctx, img)
	if err != nil || reported {
		return false, err
	}
	// ファイルを先に消す（逆順だと、記録だけ消えてファイルを見つけられなくなる）。
	if err := s.d.MessageStore.DeleteObject(ctx, img.RemovedKey); err != nil {
		return false, err
	}
	return true, s.d.Images.DeleteRemovedCourseChatImage(ctx, img.MediaID)
}

func reportTarget(source model.CourseChatImageSource, id int64) (model.ReportTargetType, string) {
	targetType := model.TargetMessage
	switch source {
	case model.CourseChatImageSourceQuestion:
		targetType = model.TargetQuestion
	case model.CourseChatImageSourceAnswer:
		targetType = model.TargetAnswer
	}
	return targetType, strconv.FormatInt(id, 10)
}

// broadcast は開いている画面へ画像の増減を伝えるために、親の更新を配信する。
// 画像の操作はもう終わったので、親を引けなくても成功のまま返す（親ごと消された場合など）。
func (s *Service) broadcast(ctx context.Context, source model.CourseChatImageSource, targetID int64) {
	var err error
	switch source {
	case model.CourseChatImageSourceMessage:
		err = s.broadcastMessage(ctx, targetID)
	case model.CourseChatImageSourceQuestion:
		var q *model.Question
		if q, err = s.d.Questions.GetQuestionByID(ctx, targetID); err == nil && q != nil {
			s.d.QuestionEvents.QuestionUpdated(ctx, q)
		}
	case model.CourseChatImageSourceAnswer:
		var a *model.Answer
		if a, err = s.d.Answers.GetAnswerByID(ctx, targetID); err == nil && a != nil {
			s.d.AnswerEvents.AnswerUpdated(ctx, a)
		}
	}
	if err != nil {
		logger.Log.Warn().Err(err).Str("component", "course_chat_image").Int64("target_id", targetID).Msg("failed to broadcast the image change")
	}
}

func (s *Service) broadcastMessage(ctx context.Context, messageID int64) error {
	msg, err := s.d.Messages.GetMessageByID(ctx, messageID)
	if err != nil || msg == nil {
		return err
	}
	room, err := s.d.GetRoom.Execute(ctx, msg.RoomID)
	if err != nil {
		return err
	}
	// ActorID は通知の「誰が」だが、AddedMentions が空なので通知は出ない（リアクションと同じ）。
	s.d.ChatEvents.MessageUpdated(ctx, chatusecase.MessageUpdatedEvent{Room: room, Message: msg, ActorID: msg.UserID})
	return nil
}
