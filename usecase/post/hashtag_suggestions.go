package post

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// PopularHashtagsCap は先読み（人気タグ一括取得）で返すタグの上限。
// タグの種類数がこの値以下なら全タグを返し、クライアントは入力中のサジェストを
// すべてローカルで解決できる（サーバー通信ゼロ）。超えた場合は人気上位のみを返し、
// 先読みに無いプレフィックスのときだけ SuggestHashtags を叩く。
const PopularHashtagsCap = 500

const (
	defaultSuggestLimit = 8
	maxSuggestLimit     = 20
)

// PopularHashtagsUseCase は人気タグの先読み用ユースケース。
// items（人気上位・最大 PopularHashtagsCap 件）と total（タグの種類数）を返す。
type PopularHashtagsUseCase interface {
	Execute(ctx context.Context) (items []*model.HashtagSuggestion, total int, err error)
}

var _ PopularHashtagsUseCase = &PopularHashtagsInteractor{}

type PopularHashtagsInteractor struct {
	postRepo repository.PostRepository
}

func NewPopularHashtagsUseCase(postRepo repository.PostRepository) PopularHashtagsUseCase {
	return &PopularHashtagsInteractor{postRepo: postRepo}
}

func (uc *PopularHashtagsInteractor) Execute(ctx context.Context) ([]*model.HashtagSuggestion, int, error) {
	items, err := uc.postRepo.ListPopularHashtags(ctx, PopularHashtagsCap)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.postRepo.CountDistinctHashtags(ctx)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// SuggestHashtagsUseCase はプレフィックス前方一致のサジェスト用ユースケース（先読みの補完）。
type SuggestHashtagsUseCase interface {
	Execute(ctx context.Context, prefix string, limit int) ([]*model.HashtagSuggestion, error)
}

var _ SuggestHashtagsUseCase = &SuggestHashtagsInteractor{}

type SuggestHashtagsInteractor struct {
	postRepo repository.PostRepository
}

func NewSuggestHashtagsUseCase(postRepo repository.PostRepository) SuggestHashtagsUseCase {
	return &SuggestHashtagsInteractor{postRepo: postRepo}
}

func (uc *SuggestHashtagsInteractor) Execute(ctx context.Context, prefix string, limit int) ([]*model.HashtagSuggestion, error) {
	// 先頭の "#" やスペースが付いていても受け付ける。
	prefix = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(prefix), "#"))
	if prefix == "" {
		return []*model.HashtagSuggestion{}, nil
	}

	if limit <= 0 {
		limit = defaultSuggestLimit
	}
	if limit > maxSuggestLimit {
		limit = maxSuggestLimit
	}

	return uc.postRepo.SuggestHashtagsByPrefix(ctx, prefix, limit)
}
