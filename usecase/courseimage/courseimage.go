// Package courseimage は授業内チャットに上がった画像の一元管理（ADM-05）。
// メッセージ・質問・回答の画像を管理画面でまとめて一覧し、削除できる。
// 理由: 授業資料の共有が横行するのを防ぐため。
package courseimage

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/upload"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type Service struct {
	images repository.CourseChatImageRepository
	// public は質問・回答の添付、messages はメッセージの添付の置き場。
	public   repository.StorageRepository
	messages repository.StorageRepository
}

func NewService(images repository.CourseChatImageRepository, public, messages repository.StorageRepository) *Service {
	return &Service{images: images, public: public, messages: messages}
}

// storeFor は画像の実体がある置き場を返す。メッセージの添付は非公開の置き場にあり、分離前の
// 添付だけが公開側に残っている（graph の messageMediaURL と同じ見分け方）。
func (s *Service) storeFor(img *model.CourseChatImage) repository.StorageRepository {
	if img.Source != model.CourseChatImageSourceMessage {
		return s.public
	}
	if kind, ok := upload.KindForObjectKey(img.Media.StorageKey); ok && !kind.Private {
		return s.public
	}
	return s.messages
}

func (s *Service) List(ctx context.Context, filter repository.CourseChatImageFilter, limit, offset int) ([]*model.CourseChatImage, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return s.images.ListCourseChatImages(ctx, filter, limit, offset)
}

// Delete は授業内チャットの画像を消す。ファイルを先に消し、
// そのあとで記録（media 行と添付の紐付け）を消す。ファイルの削除に失敗したらエラーで返し、
// 記録は残すので、やり直せる（逆順だと、記録だけ消えてファイルが公開されたまま残る）。
// 授業内チャットの画像でないメディア（アイコンなど）は、この経路では消せない。
func (s *Service) Delete(ctx context.Context, mediaID int64) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	img, err := s.images.GetCourseChatImage(ctx, mediaID)
	if err != nil {
		return err
	}
	if img == nil {
		return apperr.NotFound("画像が見つかりません")
	}
	if err := s.storeFor(img).DeleteObject(ctx, img.Media.StorageKey); err != nil {
		logger.Log.Error().Err(err).Str("component", "course_chat_image").Int64("media_id", mediaID).Msg("failed to delete the stored object")
		return err
	}
	if err := s.images.DeleteCourseChatImage(ctx, mediaID); err != nil {
		return err
	}
	audit.LogCourseChatImageDeleted(ctx, mediaID, string(img.Source), img.TargetID, img.Uploader.ID)
	return nil
}
