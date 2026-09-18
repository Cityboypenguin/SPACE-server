package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/99designs/gqlgen/graphql"
	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/vektah/gqlparser/v2/ast"
)

// チャット表示まわりで「取れなくても画面は返す」取得の失敗ログに使う lookup 名。
// 配信側の chatDelivery* と同じく、grep する側が経路で絞れるよう1箇所に集める。
const (
	chatLookupCourseRoom      = "course_room_lookup"
	chatLookupAnonymousLabel  = "anonymous_label_lookup"
	chatLookupQuestionRoom    = "question_room_lookup"
	chatLookupPollAuthorRole  = "poll_author_role_lookup"
	chatLookupBlockRelation   = "block_relation_lookup"
	chatLookupBlockedUserIDs  = "blocked_user_ids_lookup"
	chatLookupLastMessages    = "last_messages_lookup"
	chatLookupReadStatus      = "read_status_lookup"
	chatLookupReadStatusBatch = "read_status_batch_lookup"
)

// logChatLookup はリゾルバ側のベストエフォートな取得失敗を、必ず同じ形で残す。
//
// 配信側の logChatDelivery（graph/chat_events.go）と対になる口。どちらも
// 「失敗しても処理は続ける」場所なので、黙って落とすと DB 障害が
// 「未読0」「実名表示」といった正常に見える表示へ化けて誰も気づけない。
// 呼び出し側は room_id / message_id など後から追える情報を足してから Msg すること。
func logChatLookup(err error, lookup string) *zerolog.Event {
	return logger.Log.Error().Err(err).
		Str("component", "chat_resolver").
		Str("lookup", lookup)
}

// roomReadStatusFields は Room の既読まわりのフィールド名。
//
// 4つとも ChatReads の1回の取得から埋まるので、「どれか1つでも選ばれていれば
// 引く」という判定になる。並べる場所を1つにしておかないと、フィールドを足した
// ときに片方の一覧だけ取りこぼす（unreadCount は出るのに lastReadMessageID が
// 常に null、のような気づきにくい壊れ方をする）。
var roomReadStatusFields = []string{"lastReadAt", "lastReadMessageID", "unreadCount", "partnerLastReadAt"}

// roomReadStatusRequested は既読まわりのどれかが選ばれたかを返す。
// prefix は解決中のフィールドから Room までの道のり（一覧なら "items"、
// room クエリのように Room を直接返すなら省略）。
func roomReadStatusRequested(ctx context.Context, prefix ...string) bool {
	for _, name := range roomReadStatusFields {
		path := make([]string, 0, len(prefix)+1)
		path = append(path, prefix...)
		path = append(path, name)
		if fieldRequested(ctx, path...) {
			return true
		}
	}
	return false
}

// applyRoomReadStatus は既読状態を GraphQL の Room へ写す。
//
// room クエリ・myDMRooms・（将来の一覧）で同じ4項目を並べていたので1箇所にまとめた。
// 特に lastReadMessageID は、どの経路で取った Room かによって入ったり入らなかったり
// すると、クライアントの未読ページ取得の起点が ID と時刻で揺れてしまう。
//
// helpers.go に置くのは、gqlgen generate がリゾルバファイル内の「リゾルバでない
// 関数」をコメントアウトしてしまうため（anonymousUserForCourseRoom と同じ理由）。
func applyRoomReadStatus(gqlRoom *gqlmodel.Room, status *roomusecase.RoomReadStatus) {
	if gqlRoom == nil || status == nil {
		return
	}
	if status.LastReadAt != nil {
		s := time.Unix(*status.LastReadAt, 0).Format(timeFormat)
		gqlRoom.LastReadAt = &s
	}
	if status.LastReadMessageID != nil {
		id := encodeGraphID("message", *status.LastReadMessageID)
		gqlRoom.LastReadMessageID = &id
	}
	gqlRoom.UnreadCount = int32(status.UnreadCount)
	if status.PartnerLastReadAt != nil {
		s := time.Unix(*status.PartnerLastReadAt, 0).Format(timeFormat)
		gqlRoom.PartnerLastReadAt = &s
	}
}

func (r *Resolver) avatarURLFor(p *model.Profile) *string {
	if p == nil || p.AvatarMedia == nil {
		return nil
	}
	url := r.StorageRepository.PublicURL(p.AvatarMedia.StorageKey)
	return &url
}

func (r *Resolver) communityAvatarURL(c *model.Community) string {
	if c == nil || c.AvatarMedia == nil {
		return ""
	}
	return r.StorageRepository.PublicURL(c.AvatarMedia.StorageKey)
}

// anonymousUserForCourseRoom returns the synthetic User to display for authorUserID
// when roomID is a course-type room (F-05), or nil for every other room type so the
// caller falls back to showing the real user. Used by the message/question/answer/poll
// user field resolvers. This lives in helpers.go (not schema.resolvers.go) because
// gqlgen comments out any function in the resolver file that isn't a recognized
// resolver stub on every `gqlgen generate` run.
//
// 表示は読み取りだけで完結させる。匿名IDの採番は投稿時（usecase/chat・質問・
// 回答・投票の作成時）に済ませてあるので、ここでは Get しか呼ばない。以前は
// 表示時に GetOrCreate を呼んでいて、読むだけのクエリが DB に書き込む副作用を
// 持っていた。
//
// 行が引けなかった場合でも実名にはフォールバックせず、番号なしの「匿名」を返す
// （理由は anonymousPlaceholderUser のコメント）。
//
// ルームも匿名IDも DataLoader 経由で引く。理由は2つある。
//   - 一覧（メッセージ・質問・回答・投票）では項目ごとにこの関数が呼ばれるので、
//     直接引くと項目数ぶんのクエリになる。
//   - Message は userID と user の2フィールドが同じ解決を通るので、1メッセージで
//     2回引いていた。DataLoader のリクエスト内キャッシュで両方とも1回に畳まれ、
//     しかも必ず同じ匿名IDが返る（別々に引くと途中で採番されたときに食い違う）。
//
// 負のキャッシュ（行が無い、を覚えてしまうこと）は問題にならない。この関数に渡る
// のは常に「その部屋に投稿した人」で、投稿時に必ず採番されている（usecase/chat の
// ensureAnonymousIdentity ほか）ため、行が無いのは採番前の古いデータだけ。
func (r *Resolver) anonymousUserForCourseRoom(ctx context.Context, roomID string, authorUserID int64) *gqlmodel.User {
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil
	}

	room, err := dataloader.For(ctx).RoomLoader.Load(ctx, rid)
	if err != nil || room == nil {
		// ルーム種別が引けないと「授業ルームではない」と同じ扱いになり、匿名の
		// はずの投稿者が実名で出る。表示を落とすほどではないので nil を返して
		// 従来どおり続けるが、匿名性に関わる失敗なので黙って捨てない。
		// ローダーは不存在を nil で返す（エラーにしない）ので、両方ここで拾う。
		logChatLookup(err, chatLookupCourseRoom).
			Int64("room_id", rid).
			Int64("author_user_id", authorUserID).
			Msg("failed to load the room; falling back to showing the real user")
		return nil
	}
	if room.Type != model.RoomTypeCourse {
		return nil
	}

	identity, err := dataloader.For(ctx).AnonymousIdentityLoader.Load(ctx, repository.RoomUserKey{RoomID: rid, UserID: authorUserID})
	if err != nil {
		logChatLookup(err, chatLookupAnonymousLabel).
			Int64("room_id", rid).
			Int64("author_user_id", authorUserID).
			Msg("failed to load anonymous identity for course room")
		return anonymousPlaceholderUser()
	}
	if identity == nil {
		return anonymousPlaceholderUser()
	}
	return toGraphAnonymousUser(identity)
}

// requireRoomReadAccess verifies the caller may read roomID.
//
// 判定の実体は ChatAccess.EnsureReadAccess にあり、ここはそれを呼ぶだけの薄い
// ラッパ。以前は「授業は全員閲覧可、それ以外は room_users」の判定が messages
// クエリ・room クエリ・この関数・messageSubscription・roomReadStatusUpdated へ
// 写経されていて、管理者の扱いだけ食い違っていた（負債は解消済み）。
// 質問・回答・投票のクエリ／サブスクリプションも、メッセージ系と同じこの1本を通る。
//
// 逆に「閲覧できるか」ではない判定（メンション候補・ルームへの招待や削除・
// コミュニティ権限・既読位置の書き込み）はここへ寄せていない。規則が違うものを
// 同じ関数にまとめると、片方を緩めたときにもう片方まで緩む事故が起きるため。
func (r *Resolver) requireRoomReadAccess(ctx context.Context, roomID int64) (*model.Room, error) {
	return r.ChatAccess.EnsureReadAccess(ctx, roomID)
}

// questionSubscription handles the auth/access guard and PubSub fan-out for
// room-scoped question subscriptions (added, updated), mirroring messageSubscription.
func (r *subscriptionResolver) questionSubscription(ctx context.Context, roomID, topic string) (<-chan *gqlmodel.Question, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	if _, err := r.requireRoomReadAccess(ctx, rid); err != nil {
		return nil, err
	}

	return subscribeTopic[*gqlmodel.Question](ctx, r.PubSub, topic, subscriptionScope{UserID: claims.ID, RoomID: roomID}), nil
}

// roomPollSubscription はルーム単位の投票購読（added / deleted）の認証・権限判定と
// 転送をまとめる。questionSubscription と同じ形（PollUpdated だけは投票IDから
// ルームを引く必要があるのでリゾルバ側に残してある）。
func (r *subscriptionResolver) roomPollSubscription(ctx context.Context, roomID, topic string) (<-chan *gqlmodel.Poll, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	if _, err := r.requireRoomReadAccess(ctx, rid); err != nil {
		return nil, err
	}

	return subscribeTopic[*gqlmodel.Poll](ctx, r.PubSub, topic, subscriptionScope{UserID: claims.ID, RoomID: roomID}), nil
}

// answerSubscription handles the auth/access guard and PubSub fan-out for
// question-scoped answer subscriptions (added, updated, deleted), mirroring
// questionSubscription.
func (r *subscriptionResolver) answerSubscription(ctx context.Context, questionID, topic string) (<-chan *gqlmodel.Answer, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	qid, err := decodeGraphID(ctx, "question", questionID)
	if err != nil {
		return nil, fmt.Errorf("invalid question id")
	}
	q, err := r.GetQuestionByIDUseCase.Execute(ctx, qid)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, fmt.Errorf("question not found")
	}
	if _, err := r.requireRoomReadAccess(ctx, q.RoomID); err != nil {
		return nil, err
	}

	return subscribeTopic[*gqlmodel.Answer](ctx, r.PubSub, topic, subscriptionScope{
		UserID: claims.ID,
		RoomID: encodeGraphID("room", q.RoomID),
	}), nil
}

func requireAuth(ctx context.Context) (*auth.Claims, error) {
	return authz.RequireAuth(ctx)
}

func requireAdminAuth(ctx context.Context) (*auth.Claims, error) {
	return authz.RequireAdmin(ctx)
}

func isAdminRole(role string) bool {
	return authz.IsAdminRole(role)
}

func requireSelfOrAdmin(ctx context.Context, targetUserID int64, action string) (*auth.Claims, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		audit.LogDenied(ctx, action, "user", targetUserID, "unauthorized")
		return nil, err
	}

	if claims.ID == targetUserID || isAdminRole(claims.Role) {
		return claims, nil
	}

	audit.LogDenied(ctx, action, "user", targetUserID, "forbidden")
	return nil, apperr.Forbidden("forbidden")
}

func encodeGraphID(kind string, id int64) string {
	return opaqueid.Encode(kind, id)
}

func decodeGraphID(ctx context.Context, kind string, value string) (int64, error) {
	id, err := opaqueid.Decode(kind, value)
	if err != nil {
		audit.LogProbe(ctx, "decode_id", kind, value, err.Error())
		return 0, err
	}
	return id, nil
}

// isCallerID reports whether userGraphID (an opaque "user"-kind ID, decoded before
// any per-room anonymization is applied) refers to the currently authenticated
// caller. Used to compute isMine on Message/Question/Answer so the frontend can
// tell its own posts apart even when the author is displayed anonymously.
func isCallerID(ctx context.Context, userGraphID string) bool {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return false
	}
	numericID, err := decodeGraphID(ctx, "user", userGraphID)
	if err != nil {
		return false
	}
	return numericID == claims.ID
}

const (
	// defaultPageSize はページングの既定件数。GraphQL のフィールドごとに既定値が
	// 違う（クライアントの見え方が変わるため揃えられない）ので、既定値は
	// resolvePagination の引数として呼び出し側が渡す。ここはその共通の値。
	defaultPageSize = 20
	// maxPageSize はページングの共通上限。クライアントが幾ら大きい limit を
	// 送っても、ここで頭打ちにしてから SQL の LIMIT に渡す。
	maxPageSize = 100
	// maxMessagePageSize はチャット履歴だけの上限。1画面に載る件数が一覧系より
	// 多いのでここだけ緩い。クランプの実装は resolveLimit に揃えてある。
	maxMessagePageSize = 200
)

// resolveLimit は limit を「未指定・0以下なら fallback、上限超なら upper」に正規化する。
// *int32 をそのまま int にして渡すと、負数（SQL エラー）も過大値（全件走査）も
// そのまま DB に届いてしまうため、LIMIT に渡す値は必ずここを通すこと。
func resolveLimit(limit *int32, fallback, upper int) int {
	l := fallback
	if limit != nil && *limit > 0 {
		l = int(*limit)
	}
	if l > upper {
		l = upper
	}
	return l
}

// resolvePagination はページング引数（limit/offset）を正規化する。
// limit は maxPageSize で頭打ち、offset は負数を 0 に丸める。
// fallback は limit 未指定時の件数で、フィールドごとの既定値をそのまま保つために
// 呼び出し側が渡す（多くは defaultPageSize）。
func resolvePagination(limit *int32, offset *int32, fallback int) (int, int) {
	o := 0
	if offset != nil && *offset > 0 {
		o = int(*offset)
	}
	return resolveLimit(limit, fallback, maxPageSize), o
}

// ---------------------------------------------------------------------------
// 要求されていない派生値を計算しないための共通の口
// ---------------------------------------------------------------------------
//
// 一覧・詳細のリゾルバは以前、GraphQL が何を選んだかに関係なく派生値を毎回
// 埋めていた。ページ型の total（COUNT）、ルームのメンバー・ブロック判定・既読・
// 最新メッセージ、通知の actor と対象 Post、投票の unvotedTotal、コミュニティの
// 人数と所属フラグ、授業の履修者数。items だけ欲しいクライアントにも、これら
// 全部のクエリが必ず乗っていた。
//
// 方式は「選択判定を1本だけ作り、全箇所がそれを見る」に統一した。混在を避ける
// ため、今回の対象はどれもこの fieldRequested（ページングは後述の
// resolvePageQuery 経由）だけを使うこと。
//
// ■ 採用した方式: fieldRequested による選択判定
//
//   - 親リゾルバが「このフィールドは要求されたか」をこの関数1つに聞き、不要なら
//     計算そのものを呼ばない。判定の実装が1箇所なので、フラグメントや @skip の
//     扱いが箇所ごとにずれない。
//   - 計算は従来どおり親リゾルバの中で走る。要求されたときの返り値・エラー文言・
//     エラーの path（例: ["users"]）・null の出方が現状のまま変わらない。
//   - 「数えるか否か」を下層（ユースケース／リポジトリ）まで運ぶ必要がある
//     ページングだけは、bool を引数に足さず repository.PageQuery という明示的な
//     オプション型1つに載せる。増やし方を型に固定しておくことで、一覧ごとに
//     引数の並びが食い違わない。
//
// ■ 採らなかった方式(a): gqlgen.yml で total などを field resolver にする
//
//   - gqlgen としては素直だが、total を Page 型の field resolver にすると「何を
//     数えるか」を親の解決結果に持ち回す必要がある。カウント用のクロージャを
//     持つ非公開フィールド付きのカスタムモデルが、対象17ページ型ぶん増える。
//     Room の members/既読のように複数フィールドが1回の取得を共有している所では、
//     さらにその取得結果の受け渡し先も要る。
//   - 重いのは外から見える差のほう。COUNT が失敗したときのエラーの path が
//     ["users"] から ["users","total"] へ動き、部分エラー時に total が null に
//     なる（今は一覧ごとエラー）。「外から見える挙動を変えない」を満たせない。
//   - なお gqlgen.yml に既にある resolver: true（Post.user, Message.media など）は
//     DataLoader 前提の別の話で、今回の「要求されていない派生値」とは関心事が違う。
//     既存を剥がすことも、今回の対象をそちらへ寄せることもしない。
//
// ■ 採らなかった方式: 引数フラグ（includeTotal: Boolean = true など）をスキーマに足す
//
//   - スキーマの型定義を変えないという前提に反する。加えて「total を選んでいるのに
//     引数を付け忘れると 0 が返る」という、選択と引数の二重管理をクライアントに
//     強いることになる。

// fieldRequested は、いま解決中のフィールドの下に path が選択されているかを返す。
//
// path は解決中のフィールドから見た相対パス。room クエリの直下なら
// fieldRequested(ctx, "user")、一覧の要素の中なら
// fieldRequested(ctx, "items", "user") のように書く。
//
// 判定できないとき（GraphQL の実行文脈の外 = PubSub への配信、内部呼び出し、
// テスト）は必ず true を返す。ここで false に倒すと「要求されていない」と誤判定
// して、本来出るはずの値が欠けたまま配信されてしまう。誤って計算してしまうぶん
// には従来と同じ費用が乗るだけで、外から見える挙動は変わらない。
func fieldRequested(ctx context.Context, path ...string) bool {
	if len(path) == 0 {
		return true
	}
	fc := graphql.GetFieldContext(ctx)
	oc := graphql.GetOperationContext(ctx)
	if fc == nil || oc == nil || oc.Doc == nil {
		return true
	}
	return selectionHasPath(oc, fc.Field.Selections, path)
}

// selectionHasPath は選択集合を path に沿って降りる。
//
// graphql.CollectFields に satisfies = nil を渡すと、フラグメントの型条件を問わず
// 全て畳み込んでくれる（gqlgen の doesFragmentConditionMatch がそう作られている）。
// 判定は緩い側＝「選ばれているかもしれないなら計算する」へ倒したいので、これで
// よい。@skip / @include は CollectFields 側が見てくれる（応答に出ないものは
// 計算しなくてよいので、そのまま従う）。
//
// 同じ名前が別名（alias）や別のフラグメントで複数回現れることがあるので、
// 最初に見つかった1つで打ち切らず、どれか1つでも path の続きを持っていれば
// true にする。
func selectionHasPath(oc *graphql.OperationContext, sel ast.SelectionSet, path []string) bool {
	if len(sel) == 0 {
		return false
	}
	for _, f := range graphql.CollectFields(oc, sel, nil) {
		if f.Name != path[0] {
			continue
		}
		if len(path) == 1 {
			return true
		}
		if selectionHasPath(oc, f.Selections, path[1:]) {
			return true
		}
	}
	return false
}

// resolvePageQuery は limit/offset の正規化と「total が要るか」の判定をまとめて
// 済ませ、一覧系のユースケースへ渡す1つの値にする。
//
// オフセットページングの一覧リゾルバは、resolvePagination ではなく必ずこちらを
// 通すこと。total を数えるかどうかの判定がリゾルバごとに写経されると、片方だけ
// 直し忘れて「total を選んだのに 0 が返る」事故になる。判定の実体は
// fieldRequested 1本で、ここはページ型が必ず total という名前の直下フィールドを
// 持つことに乗っているだけ（スキーマの Page 型はすべてその形）。
//
// resolvePagination は「総件数を返さない」一覧だけに残してある
// （カーソルページングの messages と、総件数を持たない adminListMediaMissingDimensions）。
// 数える／数えないの判断が要らないので、この2つは PageQuery を通さない。
// resolveAnalyticsFields は「応答に出るフィールドの名前」を集めて集計の実装まで
// 運ぶ値にする。
//
// 管理画面の集計は 35 本以上の SQL を投げるので、選ばれていないものまで走らせる
// と1画面ぶんの負荷がそのまま無駄になる。判定は fieldRequested と同じ選択集合を
// 見るが、こちらは「どれか1つ」ではなく「選ばれた名前の一覧」が要る
// （どの名前がどの SQL を要するかはリポジトリ側の対応表が持つ）。
//
// path は解決中のフィールドから、名前を数えたい型までの道のり。
// adminGetAnalytics のように直下なら省略、adminGetTimeSeries のように
// 1段挟むなら "points" を渡す。
//
// 判定できないとき（GraphQL 以外からの呼び出し）は
// repository.AllFields＝全部計算する側へ倒す。fieldRequested が
// true へ倒れるのと同じで、黙って 0 を返すよりは余計に計算するほうがましだから。
func resolveAnalyticsFields(ctx context.Context, path ...string) repository.FieldSet {
	fc := graphql.GetFieldContext(ctx)
	oc := graphql.GetOperationContext(ctx)
	if fc == nil || oc == nil || oc.Doc == nil {
		return repository.AllFields()
	}
	sel, ok := selectionAtPath(oc, fc.Field.Selections, path)
	if !ok {
		// path の途中が選ばれていない＝その型のフィールドは1つも応答に出ない。
		return repository.NewFieldSet(nil)
	}
	return repository.NewFieldSet(collectedFieldNames(oc, sel))
}

// selectionAtPath は選択集合を path に沿って降り、着いた先の選択集合を返す。
// 同じ名前が別名やフラグメントで複数回現れることがあるので、見つかった選択集合を
// すべて連結して返す（片方だけ見ると取りこぼす）。
func selectionAtPath(oc *graphql.OperationContext, sel ast.SelectionSet, path []string) (ast.SelectionSet, bool) {
	if len(path) == 0 {
		return sel, true
	}
	var merged ast.SelectionSet
	found := false
	for _, f := range graphql.CollectFields(oc, sel, nil) {
		if f.Name != path[0] {
			continue
		}
		if sub, ok := selectionAtPath(oc, f.Selections, path[1:]); ok {
			merged = append(merged, sub...)
			found = true
		}
	}
	return merged, found
}

// collectedFieldNames は選択集合の直下にあるフィールド名を返す。
// 別名（alias）で選ばれていても、集計に要るのはスキーマ上の名前なので f.Name を使う。
// @skip / @include は CollectFields 側が見てくれる（応答に出ないものは計算しなくてよい）。
func collectedFieldNames(oc *graphql.OperationContext, sel ast.SelectionSet) []string {
	fields := graphql.CollectFields(oc, sel, nil)
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}

func resolvePageQuery(ctx context.Context, limit *int32, offset *int32, fallback int) repository.PageQuery {
	l, o := resolvePagination(limit, offset, fallback)
	return repository.PageQuery{Limit: l, Offset: o, WithTotal: fieldRequested(ctx, "total")}
}

func containsInt64(slice []int64, val int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// dmPartnerID は DM のメンバー一覧から「自分ではない相手」を返す。
// 相手が1人に決まらない（退会して自分しか居ない、データ不整合で複数居る）ときは
// 0 と false を返す。
//
// ルームが DM かどうかは room.Type で判断すること。以前はここが「メンバーが2人なら
// DM」という人数からの推測で、2人だけのコミュニティまで DM 扱いになっていた
// （ブロック相手と2人のコミュニティに居ると投稿欄が閉じてしまう）。相手の特定と
// ルーム種別の判定は別の関心事なので、この関数は種別を見ない。
func dmPartnerID(users []*model.User, selfID int64) (int64, bool) {
	var partnerID int64
	found := 0
	for _, u := range users {
		if u.ID == selfID {
			continue
		}
		partnerID = u.ID
		found++
	}
	if found != 1 {
		return 0, false
	}
	return partnerID, true
}

// reportTargetKind は ReportTargetType を opaqueid のカインド文字列に変換する。
func reportTargetKind(t gqlmodel.ReportTargetType) string {
	switch t {
	case gqlmodel.ReportTargetTypePost:
		return "post"
	case gqlmodel.ReportTargetTypeUser:
		return "user"
	case gqlmodel.ReportTargetTypeCommunity:
		return "community"
	default:
		return ""
	}
}

func (r *queryResolver) buildProfile(ctx context.Context, u *model.User) (*gqlmodel.Profile, error) {
	p, err := r.GetProfileUseCase.Execute(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	return toGraphProfile(u, p, r.avatarURLFor(p)), nil
}

func (r *queryResolver) favoriteUsersToGQL(ctx context.Context, favs []*model.FavoriteUser) ([]*gqlmodel.User, error) {
	ids := make([]int64, 0, len(favs))
	for _, f := range favs {
		ids = append(ids, f.FavoriteUserID)
	}
	users, err := r.GetUsersByIDsUseCase.Execute(ctx, ids)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	result := make([]*gqlmodel.User, 0, len(favs))
	for _, f := range favs {
		if u, ok := userMap[f.FavoriteUserID]; ok {
			result = append(result, toGraphUser(u))
		}
	}
	return result, nil
}

func (r *queryResolver) followersToGQL(ctx context.Context, followers []*model.FavoriteUser) ([]*gqlmodel.User, error) {
	ids := make([]int64, 0, len(followers))
	for _, f := range followers {
		ids = append(ids, f.UserID)
	}
	users, err := r.GetUsersByIDsUseCase.Execute(ctx, ids)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	result := make([]*gqlmodel.User, 0, len(followers))
	for _, f := range followers {
		if u, ok := userMap[f.UserID]; ok {
			result = append(result, toGraphUser(u))
		}
	}
	return result, nil
}

func (r *queryResolver) blockedUsersToGQL(ctx context.Context, blockers []*model.Blocker) ([]*gqlmodel.User, error) {
	ids := make([]int64, 0, len(blockers))
	for _, b := range blockers {
		ids = append(ids, b.BlockedUserID)
	}
	users, err := r.GetUsersByIDsUseCase.Execute(ctx, ids)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	result := make([]*gqlmodel.User, 0, len(blockers))
	for _, b := range blockers {
		if u, ok := userMap[b.BlockedUserID]; ok {
			result = append(result, toGraphUser(u))
		}
	}
	return result, nil
}

// presignedImageUploadURL generates a presigned put URL for image uploads.
// prefix is the storage path prefix without trailing slash (e.g., "avatars/123").
func (r *queryResolver) presignedImageUploadURL(ctx context.Context, prefix string, maxBytes int64, contentType string) (*gqlmodel.PresignedUploadURL, error) {
	extMap := map[string]string{
		"image/jpeg":    ".jpg",
		"image/png":     ".png",
		"image/webp":    ".webp",
		"image/gif":     ".gif",
		"image/svg+xml": ".svg",
	}
	ext, ok := extMap[contentType]
	if !ok {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
	objectKey := fmt.Sprintf("%s/%s%s", prefix, uuid.New().String(), ext)
	uploadURL, err := r.StorageRepository.PresignedPutURL(ctx, objectKey, contentType, 15*time.Minute, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate upload url")
	}
	return &gqlmodel.PresignedUploadURL{UploadURL: uploadURL, ObjectKey: objectKey}, nil
}

// messageSubscription handles the common auth/membership guard and PubSub fan-out
// for room-scoped message subscriptions (added, deleted, updated).
func (r *subscriptionResolver) messageSubscription(ctx context.Context, roomID, topic string) (<-chan *gqlmodel.Message, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	// 閲覧権限は messages クエリと同じ1本（ChatAccess.EnsureReadAccess）を通す。
	if _, err := r.requireRoomReadAccess(ctx, rid); err != nil {
		return nil, err
	}

	return subscribeTopic[*gqlmodel.Message](ctx, r.PubSub, topic, subscriptionScope{UserID: claims.ID, RoomID: roomID}), nil
}

// toNullableInt32 は nil を保ったまま int を GraphQL の Int（*int32）へ変換する。
func toNullableInt32(v *int) *int32 {
	if v == nil {
		return nil
	}
	converted := int32(*v)
	return &converted
}

// toNullableInt は GraphQL の Int（*int32）を nil を保ったまま int へ変換する。
// 妥当でない申告は捨てて nil にし、寸法未取得と同じ扱いに落とす。
func toNullableInt(v *int32) *int {
	if v == nil || !model.ValidImageDimension(int(*v)) {
		return nil
	}
	converted := int(*v)
	return &converted
}

// toMediaInputs は GraphQL の添付入力をユースケース層の形へ変換する。
// 添付を受け取る全ミューテーション（投稿・編集・メッセージ・質問・回答）が同じ
// 変換をするため、ここに集約して Media の属性追加時に触る箇所を1つに保つ。
func toMediaInputs(inputs []*gqlmodel.MediaUploadInput) []model.MediaInput {
	var result []model.MediaInput
	for _, m := range inputs {
		if m == nil {
			continue
		}
		result = append(result, model.MediaInput{
			StorageKey:  m.ObjectKey,
			ContentType: m.ContentType,
			Width:       toNullableInt(m.Width),
			Height:      toNullableInt(m.Height),
		})
	}
	return result
}

// notificationTargetMessage resolves the Notification/NotificationGroup targetMessage
// field: the chat message a reply notification points at. 削除済み・対象が
// メッセージ以外なら nil を返す。
//
// 返す Message は messageResolver 経由で解決されるので、授業内チャットなら user
// フィールドは自動的に匿名表示になる。
func (r *Resolver) notificationTargetMessage(ctx context.Context, targetType *string, targetID *string) (*gqlmodel.Message, error) {
	if targetType == nil || *targetType != notificationTargetTypeMessage || targetID == nil {
		return nil, nil
	}
	numericID, err := decodeGraphID(ctx, "message", *targetID)
	if err != nil {
		return nil, nil
	}
	msg, err := dataloader.For(ctx).MessageLoader.Load(ctx, numericID)
	if err != nil || msg == nil {
		return nil, nil
	}
	return toGraphMessage(msg), nil
}

// resolveMentions は本文中のメンション（保存済み）を GraphQL の Mention に変換する。
// メンション先のユーザーは DataLoader 経由でまとめて引く。
// 退会済みなどでユーザーを引けなかったメンションは黙って落とす
// （表示側はそのメンションを通常テキストとして描画する）。
//
// ループの中で Load を1件ずつ呼んではいけない。Load はバッチが閉じるまで
// （WithWait ぶん）ブロックするので、1件目の待ちが明けてから2件目のキーが
// 入る、という直列化が起きて「メンション人数 × 待ち時間」かかる。
// LoadAll は全キーを先に同じバッチへ積んでから待つので、何人居ても待ちは1回。
func resolveMentions(ctx context.Context, mentions []*model.Mention) ([]*gqlmodel.Mention, error) {
	if len(mentions) == 0 {
		return []*gqlmodel.Mention{}, nil
	}

	userIDs := make([]int64, len(mentions))
	for i, m := range mentions {
		userIDs[i] = m.UserID
	}

	users, err := dataloader.For(ctx).UserLoader.LoadAll(ctx, userIDs)
	if err != nil {
		// LoadAll は同じ失敗を人数分束ねて返すので、1件ずつ Load していた頃と
		// 同じ文言になるよう先頭の1件だけ返す。
		return nil, dataloader.FirstError(err)
	}

	result := make([]*gqlmodel.Mention, 0, len(mentions))
	for i, m := range mentions {
		if users[i] == nil {
			continue
		}
		result = append(result, &gqlmodel.Mention{
			User: toGraphUser(users[i]),
			Text: m.Text,
		})
	}
	return result, nil
}

// decodeMentionUserIDs はクライアントから届いたメンション先の GraphQL ID を
// 数値IDへ変換する。
//
// リゾルバが担うのはこのデコードだけ。「どのルームでメンションが成立するか」
// （messageusecase.MentionsSupported: コミュニティのみ）「誰をメンションできるか」の
// 判断は全て messageusecase.ResolveMentionsUseCase 側にあり、それを呼ぶのは
// ChatCommands だけ。リゾルバから直接呼ぶ経路を残さないことで、別経路から
// 授業内チャットに実名メンションが通ることを防いでいる。
func decodeMentionUserIDs(ctx context.Context, mentionUserIDs []string) ([]int64, error) {
	if len(mentionUserIDs) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(mentionUserIDs))
	for _, encoded := range mentionUserIDs {
		id, err := decodeGraphID(ctx, "user", encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid mention user id")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
