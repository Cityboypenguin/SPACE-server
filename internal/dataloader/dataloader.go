package dataloader

import (
	"context"
	"net/http"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/vikstrous/dataloadgen"
)

type GetUsersByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.User, error)
}
type ListMediaByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Media, error)
}
type ListMediaByMessageIDsUseCase interface {
	Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Media, error)
}
type GetRepliesByPostIDsUseCase interface {
	Execute(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
}

// ⭕️ 追加：管理者用リプライ取得UseCase
type GetRepliesByPostIDsIncludeDeletedUseCase interface {
	Execute(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
}
type GetFavoritesByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Favorite, error)
}

type ctxKey string

const loadersKey = ctxKey("dataloaders")

type Loaders struct {
	UserLoader         *dataloadgen.Loader[int64, *model.User]
	MediaLoader        *dataloadgen.Loader[int64, []*model.Media]
	MessageMediaLoader *dataloadgen.Loader[int64, []*model.Media]
	ReplyLoader        *dataloadgen.Loader[int64, []*model.Post]
	AdminReplyLoader   *dataloadgen.Loader[int64, []*model.Post] // ⭕️ 追加
	FavoriteLoader     *dataloadgen.Loader[int64, []*model.Favorite]
}

// batchFromMap は「IDのスライスを受け取り map[ID]V を返す関数」を DataLoader が要求する
// 「IDと同じ長さの []V スライスを返す関数」に変換する汎用ヘルパー。
func batchFromMap[V any](
	fetch func(context.Context, []int64) (map[int64]V, error),
) func(context.Context, []int64) ([]V, []error) {
	return func(ctx context.Context, ids []int64) ([]V, []error) {
		m, err := fetch(ctx, ids)
		errs := make([]error, len(ids))
		if err != nil {
			for i := range errs {
				errs[i] = err
			}
			return nil, errs
		}
		result := make([]V, len(ids))
		for i, id := range ids {
			result[i] = m[id]
		}
		return result, errs
	}
}

func Middleware(
	getUsersUseCase GetUsersByIDsUseCase,
	listMediaUseCase ListMediaByPostIDsUseCase,
	listMessageMediaUseCase ListMediaByMessageIDsUseCase,
	getRepliesUseCase GetRepliesByPostIDsUseCase,
	getAdminRepliesUseCase GetRepliesByPostIDsIncludeDeletedUseCase, // ⭕️ 引数に追加
	getFavoritesUseCase GetFavoritesByPostIDsUseCase,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			fetchUsers := func(ctx context.Context, userIDs []int64) ([]*model.User, []error) {
				users, err := getUsersUseCase.Execute(ctx, userIDs)
				if err != nil {
					errs := make([]error, len(userIDs))
					for i := range errs {
						errs[i] = err
					}
					return nil, errs
				}
				userMap := make(map[int64]*model.User, len(users))
				for _, u := range users {
					userMap[u.ID] = u
				}
				result := make([]*model.User, len(userIDs))
				errs := make([]error, len(userIDs))
				for i, id := range userIDs {
					result[i] = userMap[id]
				}
				return result, errs
			}

			loaders := &Loaders{
				UserLoader:         dataloadgen.NewLoader(fetchUsers, dataloadgen.WithWait(10*time.Millisecond)),
				MediaLoader:        dataloadgen.NewLoader(batchFromMap(listMediaUseCase.Execute), dataloadgen.WithWait(10*time.Millisecond)),
				MessageMediaLoader: dataloadgen.NewLoader(batchFromMap(listMessageMediaUseCase.Execute), dataloadgen.WithWait(10*time.Millisecond)),
				ReplyLoader:        dataloadgen.NewLoader(batchFromMap(getRepliesUseCase.Execute), dataloadgen.WithWait(10*time.Millisecond)),
				AdminReplyLoader:   dataloadgen.NewLoader(batchFromMap(getAdminRepliesUseCase.Execute), dataloadgen.WithWait(10*time.Millisecond)), // ⭕️ 追加
				FavoriteLoader:     dataloadgen.NewLoader(batchFromMap(getFavoritesUseCase.Execute), dataloadgen.WithWait(10*time.Millisecond)),
			}

			ctx = context.WithValue(ctx, loadersKey, loaders)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func For(ctx context.Context) *Loaders {
	loaders, ok := ctx.Value(loadersKey).(*Loaders)
	if !ok {
		panic("dataloaders not found in context. Check if middleware is applied.")
	}
	return loaders
}
