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
	UserLoader       *dataloadgen.Loader[int64, *model.User]
	MediaLoader      *dataloadgen.Loader[int64, []*model.Media]
	ReplyLoader      *dataloadgen.Loader[int64, []*model.Post]
	AdminReplyLoader *dataloadgen.Loader[int64, []*model.Post] // ⭕️ 追加
	FavoriteLoader   *dataloadgen.Loader[int64, []*model.Favorite]
}

func Middleware(
	getUsersUseCase GetUsersByIDsUseCase,
	listMediaUseCase ListMediaByPostIDsUseCase,
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

			fetchMedia := func(ctx context.Context, postIDs []int64) ([][]*model.Media, []error) {
				mediaMap, err := listMediaUseCase.Execute(ctx, postIDs)
				if err != nil {
					errs := make([]error, len(postIDs))
					for i := range errs {
						errs[i] = err
					}
					return nil, errs
				}
				result := make([][]*model.Media, len(postIDs))
				errs := make([]error, len(postIDs))
				for i, id := range postIDs {
					result[i] = mediaMap[id]
				}
				return result, errs
			}

			// --- 一般ユーザー用 Reply Batch ---
			fetchReplies := func(ctx context.Context, postIDs []int64) ([][]*model.Post, []error) {
				replyMap, err := getRepliesUseCase.Execute(ctx, postIDs)
				if err != nil {
					errs := make([]error, len(postIDs))
					for i := range errs {
						errs[i] = err
					}
					return nil, errs
				}
				result := make([][]*model.Post, len(postIDs))
				errs := make([]error, len(postIDs))
				for i, id := range postIDs {
					result[i] = replyMap[id]
				}
				return result, errs
			}

			// ⭕️ 追加：管理者用 Reply Batch ---
			fetchAdminReplies := func(ctx context.Context, postIDs []int64) ([][]*model.Post, []error) {
				replyMap, err := getAdminRepliesUseCase.Execute(ctx, postIDs)
				if err != nil {
					errs := make([]error, len(postIDs))
					for i := range errs {
						errs[i] = err
					}
					return nil, errs
				}
				result := make([][]*model.Post, len(postIDs))
				errs := make([]error, len(postIDs))
				for i, id := range postIDs {
					result[i] = replyMap[id]
				}
				return result, errs
			}

			fetchFavorites := func(ctx context.Context, postIDs []int64) ([][]*model.Favorite, []error) {
				favMap, err := getFavoritesUseCase.Execute(ctx, postIDs)
				if err != nil {
					errs := make([]error, len(postIDs))
					for i := range errs {
						errs[i] = err
					}
					return nil, errs
				}
				result := make([][]*model.Favorite, len(postIDs))
				errs := make([]error, len(postIDs))
				for i, id := range postIDs {
					result[i] = favMap[id]
				}
				return result, errs
			}

			loaders := &Loaders{
				UserLoader:       dataloadgen.NewLoader(fetchUsers, dataloadgen.WithWait(10*time.Millisecond)),
				MediaLoader:      dataloadgen.NewLoader(fetchMedia, dataloadgen.WithWait(10*time.Millisecond)),
				ReplyLoader:      dataloadgen.NewLoader(fetchReplies, dataloadgen.WithWait(10*time.Millisecond)),
				AdminReplyLoader: dataloadgen.NewLoader(fetchAdminReplies, dataloadgen.WithWait(10*time.Millisecond)), // ⭕️ 追加
				FavoriteLoader:   dataloadgen.NewLoader(fetchFavorites, dataloadgen.WithWait(10*time.Millisecond)),
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
