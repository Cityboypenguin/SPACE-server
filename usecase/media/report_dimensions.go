// 表示できた画像の実寸をクライアントから受け取って記録する。
//
// 寸法はアップロード時にクライアントが申告するが、その仕組みより前に投稿された
// メディアには入っていない。サーバー側でファイルを読み直す方法もあるが、
// 画像形式ごとにデコーダを揃える必要があり、EXIF の回転のようにブラウザと
// 解釈が食い違う余地も残る。表示したブラウザ自身に測らせれば、
// 「実際に表示される寸法」がそのまま得られ、形式にも依存しない。
package media

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ReportDimensionsUseCase interface {
	Execute(ctx context.Context, mediaID int64, width, height int) error
}

type ReportDimensionsInteractor struct {
	mediaRepo repository.MediaRepository
}

func NewReportDimensionsUseCase(mediaRepo repository.MediaRepository) ReportDimensionsUseCase {
	return &ReportDimensionsInteractor{mediaRepo: mediaRepo}
}

var ErrInvalidDimensions = errors.New("invalid image dimensions")

func (uc *ReportDimensionsInteractor) Execute(ctx context.Context, mediaID int64, width, height int) error {
	if !model.ValidImageDimension(width) || !model.ValidImageDimension(height) {
		return ErrInvalidDimensions
	}
	// 未設定のときだけ記録する。先に入っている値を後から来た観測で塗り替えない。
	return uc.mediaRepo.SetMediaDimensionsIfUnset(ctx, mediaID, width, height)
}
