package dataloader

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/vikstrous/dataloadgen"
)

type GetUsersByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.User, error)
}
type GetPostsByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) ([]*model.Post, error)
}
type ListMediaByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Media, error)
}
type ListMediaByMessageIDsUseCase interface {
	Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Media, error)
}
type ListMediaByQuestionIDsUseCase interface {
	Execute(ctx context.Context, questionIDs []int64) (map[int64][]*model.Media, error)
}
type ListMediaByAnswerIDsUseCase interface {
	Execute(ctx context.Context, answerIDs []int64) (map[int64][]*model.Media, error)
}
type GetRepliesByPostIDsUseCase interface {
	Execute(ctx context.Context, parentIDs []int64, q repository.PageQuery) (map[int64][]*model.Post, error)
}

// ⭕️ 追加：管理者用リプライ取得UseCase
type GetRepliesByPostIDsIncludeDeletedUseCase interface {
	Execute(ctx context.Context, parentIDs []int64, q repository.PageQuery) (map[int64][]*model.Post, error)
}
type GetMessagesByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*model.Message, error)
}
type GetFavoritesByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Favorite, error)
}
type ListMentionsByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Mention, error)
}
type ListMentionsByMessageIDsUseCase interface {
	Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Mention, error)
}
type GetRoomsByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*model.Room, error)
}
type GetQuestionsByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*model.Question, error)
}
type GetAnswersByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*repository.AnswerWithLikes, error)
}
type ListAnswerPagesByQuestionIDsUseCase interface {
	Execute(ctx context.Context, questionIDs []int64, q repository.PageQuery) (map[int64]*repository.AnswerPage, error)
}
type ListPollOptionResultsByPollIDsUseCase interface {
	Execute(ctx context.Context, pollIDs []int64) (map[int64][]*repository.PollOptionResult, error)
}
type CountPollVotersByPollIDsUseCase interface {
	Execute(ctx context.Context, pollIDs []int64) (map[int64]int, error)
}
type GetAnonymousIdentitiesUseCase interface {
	Execute(ctx context.Context, keys []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error)
}

type ctxKey string

const loadersKey = ctxKey("dataloaders")

// AnswerPageKey は Question.answers（ページング付き）の DataLoader キー。
//
// 他のローダーと違ってIDだけでは足りない。GraphQL の answers フィールドは
// limit/offset をクライアントから受け取るので、同じ質問でも件数が違えば別の結果になる。
// comparable な構造体にしておけば DataLoader のキャッシュがそのまま正しく効く。
type AnswerPageKey struct {
	QuestionID int64
	// Page は窓と「total を数えるか」。WithTotal までキーに含めないと、total を
	// 選ばなかった問い合わせの結果（Total=0）を、同じ窓で total を選んだ側が
	// 使い回してしまう。
	Page repository.PageQuery
}

type ReplyPageKey struct {
	PostID int64
	Page   repository.PageQuery
}

type Loaders struct {
	UserLoader          *dataloadgen.Loader[int64, *model.User]
	MediaLoader         *dataloadgen.Loader[int64, []*model.Media]
	MessageMediaLoader  *dataloadgen.Loader[int64, []*model.Media]
	QuestionMediaLoader *dataloadgen.Loader[int64, []*model.Media]
	AnswerMediaLoader   *dataloadgen.Loader[int64, []*model.Media]
	ReplyLoader         *dataloadgen.Loader[ReplyPageKey, []*model.Post]
	AdminReplyLoader    *dataloadgen.Loader[ReplyPageKey, []*model.Post]
	FavoriteLoader      *dataloadgen.Loader[int64, []*model.Favorite]
	// MessageLoader は引用返信の返信先メッセージ用。削除済み・不存在は nil を返す。
	MessageLoader *dataloadgen.Loader[int64, *model.Message]
	// PostMentionLoader / MessageMentionLoader は本文中のメンション用。
	// 一覧表示で1件ずつ引くと N+1 になるためまとめて解決する。
	PostMentionLoader    *dataloadgen.Loader[int64, []*model.Mention]
	MessageMentionLoader *dataloadgen.Loader[int64, []*model.Mention]
	// PostLoader は Post.parent と Favorite.post 用。削除済み・不存在は nil を返す。
	PostLoader *dataloadgen.Loader[int64, *model.Post]
	// RoomLoader は Message.room と、授業ルームかどうかの判定（匿名表示）用。
	// 不存在は nil を返す。
	RoomLoader *dataloadgen.Loader[int64, *model.Room]
	// QuestionLoader は Answer.user が匿名表示かを決めるためのルーム特定用。
	// 回答は質問を経由しないとルームが分からないので、1画面で同じ質問を何度も引く。
	// 不存在は nil を返す。
	QuestionLoader *dataloadgen.Loader[int64, *model.Question]
	// AnswerLoader は Question.bestAnswer 用。不存在は nil を返す。
	AnswerLoader *dataloadgen.Loader[int64, *repository.AnswerWithLikes]
	// AnswerPageLoader は Question.answers 用。回答が無い質問も
	// 「空・Total 0」のページが返る（nil にはならない）。
	AnswerPageLoader *dataloadgen.Loader[AnswerPageKey, *repository.AnswerPage]
	// PollOptionLoader / PollVoterCountLoader は Poll.options / Poll.voterCount 用。
	// 選択肢が無い投票は nil スライス、投票者が居ない投票は 0 になる。
	PollOptionLoader     *dataloadgen.Loader[int64, []*repository.PollOptionResult]
	PollVoterCountLoader *dataloadgen.Loader[int64, int]
	// AnonymousIdentityLoader は授業内チャットの匿名表示名（匿名NNN）用。
	// 行が無い（＝そのルームで投稿したことがない）場合は nil を返す。
	// 呼び出し側はそこで実名へフォールバックしてはいけない。
	AnonymousIdentityLoader *dataloadgen.Loader[repository.RoomUserKey, *model.RoomAnonymousIdentity]
}

// batchFromMap は「キーのスライスを受け取り map[K]V を返す関数」を DataLoader が要求する
// 「キーと同じ長さの []V スライスを返す関数」に変換する汎用ヘルパー。
//
// map に無いキーの位置は V のゼロ値（ポインタなら nil、int なら 0）になる。
// 「見つからない」をエラーにしないのは、一覧の1件が消えていても残りは描けるようにするため。
func batchFromMap[K comparable, V any](
	fetch func(context.Context, []K) (map[K]V, error),
) func(context.Context, []K) ([]V, []error) {
	return func(ctx context.Context, keys []K) ([]V, []error) {
		m, err := fetch(ctx, keys)
		errs := make([]error, len(keys))
		if err != nil {
			for i := range errs {
				errs[i] = err
			}
			// 値のスライスはキーと同じ長さで返すこと。nil を返すと dataloadgen が
			// 「0 values returned for N keys」という自前の「bug in fetch function」
			// エラーに差し替えてしまい、本当の失敗（DB エラー等）が消える。
			// キーが1つのときだけ素通りしていたので、長らく気づかれていなかった。
			return make([]V, len(keys)), errs
		}
		result := make([]V, len(keys))
		for i, key := range keys {
			result[i] = m[key]
		}
		return result, errs
	}
}

// batchFromSlice は「IDのスライスを受け取り値のスライスを返す関数」（順不同・欠けあり）を
// DataLoader が要求する形へ変換する汎用ヘルパー。keyOf で各値のIDを取り出して詰め直す。
//
// batchFromMap と扱いを揃えてあり、返ってこなかったIDの位置は nil になる。
// 一部のリポジトリが map ではなくスライスを返すためだけに存在する（IN 句で引くので
// 並び順もリクエスト順とは限らない）。
func batchFromSlice[V any](
	fetch func(context.Context, []int64) ([]V, error),
	keyOf func(V) int64,
) func(context.Context, []int64) ([]V, []error) {
	return func(ctx context.Context, ids []int64) ([]V, []error) {
		values, err := fetch(ctx, ids)
		errs := make([]error, len(ids))
		if err != nil {
			for i := range errs {
				errs[i] = err
			}
			// 長さを揃える理由は batchFromMap のコメントと同じ。
			return make([]V, len(ids)), errs
		}
		byID := make(map[int64]V, len(values))
		for _, v := range values {
			byID[keyOf(v)] = v
		}
		result := make([]V, len(ids))
		for i, id := range ids {
			result[i] = byID[id]
		}
		return result, errs
	}
}

// batchAnswerPages は AnswerPageKey のバッチを limit/offset ごとに束ね直してから引く。
//
// SQL の LIMIT は「質問ごとの上位 N 件」を表現できないので、1回のクエリで混ぜられるのは
// limit/offset が同じキーだけ。実際には1つの GraphQL クエリの answers フィールドは
// 引数が1組なので、ほぼ必ず1グループ＝1回のクエリになる。
// （別々の引数で2回問い合わせた場合だけ2クエリに割れる。それでも質問の件数には比例しない。）
func batchAnswerPages(
	uc ListAnswerPagesByQuestionIDsUseCase,
) func(context.Context, []AnswerPageKey) ([]*repository.AnswerPage, []error) {
	return func(ctx context.Context, keys []AnswerPageKey) ([]*repository.AnswerPage, []error) {
		errs := make([]error, len(keys))

		grouped := make(map[repository.PageQuery][]int64)
		for _, k := range keys {
			grouped[k.Page] = append(grouped[k.Page], k.QuestionID)
		}

		pages := make(map[AnswerPageKey]*repository.AnswerPage, len(keys))
		for page, questionIDs := range grouped {
			m, err := uc.Execute(ctx, questionIDs, page)
			if err != nil {
				// 1グループでも失敗したら全キーを同じエラーで落とす。
				// batchFromMap と同じ倒し方（部分的な成功を返さない）。
				for i := range errs {
					errs[i] = err
				}
				// 長さを揃える理由は batchFromMap のコメントと同じ。
				return make([]*repository.AnswerPage, len(keys)), errs
			}
			for questionID, answerPage := range m {
				pages[AnswerPageKey{QuestionID: questionID, Page: page}] = answerPage
			}
		}

		result := make([]*repository.AnswerPage, len(keys))
		for i, k := range keys {
			result[i] = pages[k]
		}
		return result, errs
	}
}

func batchReplyPages(
	fetch func(context.Context, []int64, repository.PageQuery) (map[int64][]*model.Post, error),
) func(context.Context, []ReplyPageKey) ([][]*model.Post, []error) {
	return func(ctx context.Context, keys []ReplyPageKey) ([][]*model.Post, []error) {
		errs := make([]error, len(keys))
		grouped := make(map[repository.PageQuery][]int64)
		for _, key := range keys {
			grouped[key.Page] = append(grouped[key.Page], key.PostID)
		}

		pages := make(map[ReplyPageKey][]*model.Post, len(keys))
		for page, postIDs := range grouped {
			items, err := fetch(ctx, postIDs, page)
			if err != nil {
				for i := range errs {
					errs[i] = err
				}
				return make([][]*model.Post, len(keys)), errs
			}
			for postID, replies := range items {
				pages[ReplyPageKey{PostID: postID, Page: page}] = replies
			}
		}

		result := make([][]*model.Post, len(keys))
		for i, key := range keys {
			result[i] = pages[key]
		}
		return result, errs
	}
}

// UseCases は Middleware が必要とするバッチ取得の口をまとめたもの。
//
// 引数を1つずつ並べる形はやめてある。ローダーが増えるほど並びが長くなるうえ、
// ListMediaBy*IDsUseCase のようにメソッドの形が同じインターフェースは Go の
// 構造的型付けでは取り違えてもコンパイルが通ってしまい、取り違えに気づけない。
// 名前付きフィールドなら渡し間違いがそのまま読める。
type UseCases struct {
	GetUsersByIDs                  GetUsersByIDsUseCase
	GetPostsByIDs                  GetPostsByIDsUseCase
	ListMediaByPostIDs             ListMediaByPostIDsUseCase
	ListMediaByMessageIDs          ListMediaByMessageIDsUseCase
	ListMediaByQuestionIDs         ListMediaByQuestionIDsUseCase
	ListMediaByAnswerIDs           ListMediaByAnswerIDsUseCase
	GetRepliesByPostIDs            GetRepliesByPostIDsUseCase
	GetRepliesByPostIDsIncludeDel  GetRepliesByPostIDsIncludeDeletedUseCase
	GetFavoritesByPostIDs          GetFavoritesByPostIDsUseCase
	GetMessagesByIDs               GetMessagesByIDsUseCase
	ListMentionsByPostIDs          ListMentionsByPostIDsUseCase
	ListMentionsByMessageIDs       ListMentionsByMessageIDsUseCase
	GetRoomsByIDs                  GetRoomsByIDsUseCase
	GetQuestionsByIDs              GetQuestionsByIDsUseCase
	GetAnswersByIDs                GetAnswersByIDsUseCase
	ListAnswerPagesByQuestionIDs   ListAnswerPagesByQuestionIDsUseCase
	ListPollOptionResultsByPollIDs ListPollOptionResultsByPollIDsUseCase
	CountPollVotersByPollIDs       CountPollVotersByPollIDsUseCase
	GetAnonymousIdentities         GetAnonymousIdentitiesUseCase
}

// batchWait は全ローダー共通のバッチ待ち時間。
// リゾルバは並行に走るので、この窓の間に集まったキーが1クエリにまとまる。
const batchWait = 10 * time.Millisecond

// maxBatchKeys は1バッチに載せるキー数の上限。
//
// 以前は無制限（dataloadgen の既定）だった。キーはそのまま IN 句のプレースホルダに
// なるので、上限が無いと1クエリのプレースホルダ数＝集まったキー数になる。MySQL の
// プリペアドステートメントは 65535 個までしかパラメータを持てず、しかも IN 句には
// ブロック除外リスト（AppendBlockFilter）や閲覧者IDなどが同じクエリに相乗りする。
// 「キーが集まりすぎた1回」だけが Prepared statement contains too many placeholders
// で落ちる、という再現しにくい壊れ方をする。
//
// 値は 1000。根拠は2つ:
//   - プレースホルダ上限（65535）に対して十分な余裕を取る。相乗りする条件が
//     数千個あっても届かない。バルク INSERT 側の maxBulkInsertParams（60000）が
//     「上限ぎりぎりまで詰める」側なのに対し、こちらは1クエリが返す行数も
//     抑えたいので桁を落としてある。
//   - 画面から来るキー数はページングで頭打ち（maxPageSize=100、チャットでも 200）
//     なので、まともな応答では分割は起きない。効くのは「投稿100件×返信×メンション」
//     のように積が膨らんだときだけで、そこでは分割したほうが1クエリあたりの
//     行数も抑えられる。
//
// 上限に当たったバッチはその場で発火し、残りは次のバッチへ回る（dataloadgen の
// WithBatchCapacity）。キーが落ちることはないので、呼び出し側から見た結果は同じ。
const maxBatchKeys = 1000

// loaderOptions は全ローダー共通の設定。
//
// ローダーごとに列挙すると、新しいローダーを足した人が上限だけ付け忘れる
// （そのローダーだけ無制限に戻る）ので、1箇所に束ねて全員が同じものを使う。
var loaderOptions = []dataloadgen.Option{
	dataloadgen.WithWait(batchWait),
	dataloadgen.WithBatchCapacity(maxBatchKeys),
}

// New builds a fresh set of loaders. リクエストごとに作り直すこと。
// DataLoader のキャッシュはリクエスト内でだけ正しい（作り置きすると古い値を返す）。
func New(uc UseCases) *Loaders {
	return &Loaders{
		UserLoader:          dataloadgen.NewLoader(batchFromSlice(uc.GetUsersByIDs.Execute, func(u *model.User) int64 { return u.ID }), loaderOptions...),
		MediaLoader:         dataloadgen.NewLoader(batchFromMap(uc.ListMediaByPostIDs.Execute), loaderOptions...),
		MessageMediaLoader:  dataloadgen.NewLoader(batchFromMap(uc.ListMediaByMessageIDs.Execute), loaderOptions...),
		QuestionMediaLoader: dataloadgen.NewLoader(batchFromMap(uc.ListMediaByQuestionIDs.Execute), loaderOptions...),
		AnswerMediaLoader:   dataloadgen.NewLoader(batchFromMap(uc.ListMediaByAnswerIDs.Execute), loaderOptions...),
		ReplyLoader:         dataloadgen.NewLoader(batchReplyPages(uc.GetRepliesByPostIDs.Execute), loaderOptions...),
		AdminReplyLoader:    dataloadgen.NewLoader(batchReplyPages(uc.GetRepliesByPostIDsIncludeDel.Execute), loaderOptions...),
		FavoriteLoader:      dataloadgen.NewLoader(batchFromMap(uc.GetFavoritesByPostIDs.Execute), loaderOptions...),
		MessageLoader:       dataloadgen.NewLoader(batchFromMap(uc.GetMessagesByIDs.Execute), loaderOptions...),

		PostMentionLoader:    dataloadgen.NewLoader(batchFromMap(uc.ListMentionsByPostIDs.Execute), loaderOptions...),
		MessageMentionLoader: dataloadgen.NewLoader(batchFromMap(uc.ListMentionsByMessageIDs.Execute), loaderOptions...),

		PostLoader:              dataloadgen.NewLoader(batchFromSlice(uc.GetPostsByIDs.Execute, func(p *model.Post) int64 { return p.ID }), loaderOptions...),
		RoomLoader:              dataloadgen.NewLoader(batchFromMap(uc.GetRoomsByIDs.Execute), loaderOptions...),
		QuestionLoader:          dataloadgen.NewLoader(batchFromMap(uc.GetQuestionsByIDs.Execute), loaderOptions...),
		AnswerLoader:            dataloadgen.NewLoader(batchFromMap(uc.GetAnswersByIDs.Execute), loaderOptions...),
		AnswerPageLoader:        dataloadgen.NewLoader(batchAnswerPages(uc.ListAnswerPagesByQuestionIDs), loaderOptions...),
		PollOptionLoader:        dataloadgen.NewLoader(batchFromMap(uc.ListPollOptionResultsByPollIDs.Execute), loaderOptions...),
		PollVoterCountLoader:    dataloadgen.NewLoader(batchFromMap(uc.CountPollVotersByPollIDs.Execute), loaderOptions...),
		AnonymousIdentityLoader: dataloadgen.NewLoader(batchFromMap(uc.GetAnonymousIdentities.Execute), loaderOptions...),
	}
}

func Middleware(uc UseCases) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), loadersKey, New(uc))
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

// WithLoaders は ctx にローダーを差し込む。テストから For(ctx) を使うリゾルバを
// 呼ぶための口で、本番経路は Middleware が同じことをする。
func WithLoaders(ctx context.Context, loaders *Loaders) context.Context {
	return context.WithValue(ctx, loadersKey, loaders)
}

// FirstError は LoadAll が返すエラーから最初の1件を取り出す。
//
// LoadAll はキーごとのエラーを ErrorSlice に束ねて返す。バッチ関数は失敗を
// 全キーに配るので、そのまま呼び出し側へ返すと同じ文言がキーの数だけ並んだ
// エラーになる。1件ずつ Load していた頃と同じ見え方を保つために、
// 最初の非 nil だけを返す。ErrorSlice でないエラーはそのまま返す。
func FirstError(err error) error {
	var errs dataloadgen.ErrorSlice
	if errors.As(err, &errs) {
		for _, e := range errs {
			if e != nil {
				return e
			}
		}
	}
	return err
}
