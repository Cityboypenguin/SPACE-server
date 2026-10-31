package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// NGWordRepository はNGワードの登録と、止めた回数の記録。
type NGWordRepository interface {
	ListNGWords(ctx context.Context) ([]*model.NGWord, error)
	// CreateNGWord は登録する。同じ照合のしかた・同じ文字列がすでにあれば ErrDuplicateKey。
	CreateNGWord(ctx context.Context, w *model.NGWord) error
	// DeleteNGWord は消す。無ければ false。止めた回数の記録は残る。
	DeleteNGWord(ctx context.Context, id int64) (bool, error)
	RecordNGWordHit(ctx context.Context, hit *model.NGWordHit) error
	// NGWordHitStats は止めた回数の集計。ByWord・ByUser は多い順に topN 件まで。
	NGWordHitStats(ctx context.Context, topN int) (*model.NGWordHitStats, error)
}
