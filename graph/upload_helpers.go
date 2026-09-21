package graph

import (
	"context"
	"fmt"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/upload"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// presignedUploadURL generates a presigned put URL for one upload kind.
//
// 上限も受け付ける Content-Type も kind（internal/upload）から取る。ここへ数字を
// 直接書くと、受け入れ時の検査（upload.Verify）と食い違う。食い違いは「URLは
// 出るのに保存できない」という形で出るので、書いた本人には気づきにくい。
//
// ownerSegment はキーの2段目（利用者ごとに分けるため）。空なら省く。
func (r *queryResolver) presignedUploadURL(ctx context.Context, kind upload.Kind, ownerSegment string, contentType string) (*gqlmodel.PresignedUploadURL, error) {
	// 署名付きURLを出すのは staging のキーだけ。アプリが参照するキーに対して
	// URLを出すと、そのキーの中身は有効期間のあいだ差し替え可能になる
	// （internal/upload のパッケージコメント参照）。受け入れ時に写した先が
	// 最終キーになるので、クライアントはここで受け取ったキーをそのまま
	// ミューテーションへ渡せばよい（キーの形は知らなくてよい）。
	objectKey, err := kind.NewStagingKey(ownerSegment, contentType)
	if err != nil {
		return nil, err
	}
	uploadURL, err := r.StorageRepository.PresignedPutURL(ctx, objectKey, contentType, 15*time.Minute, kind.MaxBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate upload url")
	}
	return &gqlmodel.PresignedUploadURL{UploadURL: uploadURL, ObjectKey: objectKey}, nil
}

// toMediaInputs は GraphQL の添付入力をユースケース層の形へ変換する。
//
// 変換するだけで、受け入れ（上限・種別の検査と、署名付きURLの及ばないキーへの
// 移送）はしない。それはキーを保存するユースケース側の仕事になった
// （usecase/upload のパッケージコメント参照）。ここで通してしまうと、
// ユースケースを直接呼ぶ経路を足したときに検査を飛ばせる形に戻る。
func (r *Resolver) toMediaInputs(inputs []*gqlmodel.MediaUploadInput) []model.MediaInput {
	var result []model.MediaInput
	for _, m := range inputs {
		if m == nil {
			continue
		}
		result = append(result, model.MediaInput{
			StorageKey: m.ObjectKey,
			// ContentType は利用者の申告ではなく、実際に置かれたものを使いたいが、
			// ここを変えると既存レコードとの整合が要る。申告と実物の食い違いは
			// 受け入れ（internal/upload）が弾くので、保存された時点で両者は
			// 同じ種別（画像なら画像）に収まっている。
			ContentType: m.ContentType,
			Width:       toNullableInt(m.Width),
			Height:      toNullableInt(m.Height),
		})
	}
	return result
}
