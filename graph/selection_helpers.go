package graph

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/vektah/gqlparser/v2/ast"
)

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
