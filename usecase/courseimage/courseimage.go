// Package courseimage は授業内チャットに上がった画像の一元管理（ADM-05）。
// メッセージ・質問・回答の画像を管理画面でまとめて一覧し、削除できる。
// 理由: 授業資料の共有が横行するのを防ぐため。
package courseimage

import (
	"context"
	"errors"
	"strconv"

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
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// Deps は Service の依存。配信まわり（Messages 以降）は開いている画面から画像を消すために使う。
type Deps struct {
	Images repository.CourseChatImageRepository
	// Reports は通報時点の写しが指している添付を付け替えるために使う。
	Reports repository.ReportedMediaRepository
	// Public は質問・回答の添付、MessageStore はメッセージの添付の置き場。
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

type Service struct {
	d Deps
}

func NewService(d Deps) *Service {
	return &Service{d: d}
}

// storeFor は画像の実体がある置き場を返す。メッセージの添付は非公開の置き場にあり、分離前の
// 添付だけが公開側に残っている（graph の messageMediaURL と同じ見分け方）。
func (s *Service) storeFor(img *model.CourseChatImage) repository.StorageRepository {
	if img.Source != model.CourseChatImageSourceMessage {
		return s.d.Public
	}
	if kind, ok := upload.KindForObjectKey(img.Media.StorageKey); ok && !kind.Private {
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

// Delete は授業内チャットの画像を消す。ファイルを先に消し、
// そのあとで記録（media 行と添付の紐付け）を消す。ファイルの削除に失敗したらエラーで返し、
// 記録は残すので、やり直せる（逆順だと、記録だけ消えてファイルが公開されたまま残る）。
// 授業内チャットの画像でないメディア（アイコンなど）は、この経路では消せない。
//
// 通報の写しが指している画像は、対応する側が通報された時点の内容を確認できるよう、
// 利用者の知らない別のキーへ写してから元を消す（retainForReports 参照）。
func (s *Service) Delete(ctx context.Context, mediaID int64) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	img, err := s.d.Images.GetCourseChatImage(ctx, mediaID)
	if err != nil {
		return err
	}
	if img == nil {
		return apperr.NotFound("画像が見つかりません")
	}
	if err := s.retainForReports(ctx, img); err != nil {
		logger.Log.Error().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to retain the image for reports")
		return err
	}
	if err := s.storeFor(img).DeleteObject(ctx, img.Media.StorageKey); err != nil {
		logger.Log.Error().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to delete the stored object")
		return err
	}
	if err := s.d.Images.DeleteCourseChatImage(ctx, mediaID); err != nil {
		return err
	}
	audit.LogCourseChatImageDeleted(ctx, mediaID, string(img.Source), img.TargetID, img.Uploader.ID)
	s.broadcast(ctx, img)
	return nil
}

// retainForReports は、通報の写しが指している画像を新しいキーへ写し、写しの参照を付け替える。
//
// 新しいキーは写しの中（暗号化済み）にしか現れないので、元のURLを知っている利用者からは
// 見えなくなり、通報の詳細からだけ見える。写し先を引き直すだけなので置き場も種別も変わらず、
// URL の組み立ては今までのまま使える。
//
// 途中で失敗したらエラーで返し、元の画像も消さない。付け替えのあとで元のファイルの削除に
// 失敗しても、やり直しでは参照が残っていないのでここは素通りする。
func (s *Service) retainForReports(ctx context.Context, img *model.CourseChatImage) error {
	targetType, targetID := reportTargetOf(img)
	key := img.Media.StorageKey
	referenced, err := s.d.Reports.HasReportedMedia(ctx, targetType, targetID, key)
	if err != nil || !referenced {
		return err
	}
	store := s.storeFor(img)
	info, err := store.StatObject(ctx, key)
	if errors.Is(err, repository.ErrObjectNotFound) {
		// 実体がもう無ければ写せない。残っている記録を消せるように先へ進める。
		logger.Log.Warn().Str("component", "course_chat_image").Int64("media_id", img.Media.ID).Msg("the reported image has no stored object")
		return nil
	}
	if err != nil {
		return err
	}
	retainedKey, err := upload.RetainedKeyFor(key)
	if err != nil {
		return err
	}
	if err := store.CopyObject(ctx, key, retainedKey, info.ETag, img.Media.ContentType); err != nil {
		return err
	}
	if err := s.d.Reports.RepointReportedMedia(ctx, targetType, targetID, key, retainedKey); err != nil {
		// 写しはどこからも指されない。消せなくても害は無いので、元のエラーを返す。
		if delErr := store.DeleteObject(ctx, retainedKey); delErr != nil {
			logger.Log.Warn().Err(delErr).Str("component", "course_chat_image").Int64("media_id", img.Media.ID).Msg("failed to clean up the unused copy")
		}
		return err
	}
	return nil
}

func reportTargetOf(img *model.CourseChatImage) (model.ReportTargetType, string) {
	targetType := model.TargetMessage
	switch img.Source {
	case model.CourseChatImageSourceQuestion:
		targetType = model.TargetQuestion
	case model.CourseChatImageSourceAnswer:
		targetType = model.TargetAnswer
	}
	return targetType, strconv.FormatInt(img.TargetID, 10)
}

// broadcast は開いている画面から画像を消すために、親の更新を配信する。
// 画像はもう消えたので、親を引けなくても削除は成功のまま返す（親ごと消された場合など）。
func (s *Service) broadcast(ctx context.Context, img *model.CourseChatImage) {
	var err error
	switch img.Source {
	case model.CourseChatImageSourceMessage:
		err = s.broadcastMessage(ctx, img.TargetID)
	case model.CourseChatImageSourceQuestion:
		var q *model.Question
		if q, err = s.d.Questions.GetQuestionByID(ctx, img.TargetID); err == nil && q != nil {
			s.d.QuestionEvents.QuestionUpdated(ctx, q)
		}
	case model.CourseChatImageSourceAnswer:
		var a *model.Answer
		if a, err = s.d.Answers.GetAnswerByID(ctx, img.TargetID); err == nil && a != nil {
			s.d.AnswerEvents.AnswerUpdated(ctx, a)
		}
	}
	if err != nil {
		logger.Log.Warn().Err(err).Str("component", "course_chat_image").Int64("target_id", img.TargetID).Msg("failed to broadcast the image deletion")
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
