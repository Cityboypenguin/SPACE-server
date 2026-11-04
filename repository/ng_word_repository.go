package repository

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// NGWordRepository はNGワードの登録と、止めた回数の記録。
type NGWordRepository interface {
	ListNGWords(ctx context.Context) ([]*model.NGWord, error)
	// CreateNGWord は、登録数が max に達していなければ登録する（数の確認と登録は1つの
	// 文で行うので、同時に登録しても max を超えない）。達していれば ErrNGWordLimit、
	// 同じ照合のしかた・同じ文字列がすでにあれば ErrDuplicateKey。
	CreateNGWord(ctx context.Context, w *model.NGWord, max int) error
	// DeleteNGWord は消す。無ければ false。止めた回数の記録は残る。
	DeleteNGWord(ctx context.Context, id int64) (bool, error)
	RecordNGWordHit(ctx context.Context, hit *model.NGWordHit) error
	// NGWordHitStats は止めた回数の集計。ByWord・ByUser は多い順に topN 件まで。
	NGWordHitStats(ctx context.Context, topN int) (*model.NGWordHitStats, error)
}

// ErrNGWordLimit は、登録できるNGワードの数の上限に達していることを表す。
var ErrNGWordLimit = errors.New("ng word limit reached")
