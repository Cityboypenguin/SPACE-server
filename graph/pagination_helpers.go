package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

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
	// unpagedCollectionCap は「まだページングを持たないコレクション」の安全弁。
	//
	// 返信一覧・コミュニティメンバー・Favorite/Block の一覧・管理者の規約一覧は、
	// もともと引数を1つも取らず**該当する全行**を返していた。行数を決めるのが
	// データの育ち方だけなので、投稿が伸びた・コミュニティが大きくなった日に
	// 1回のクエリが重くなる（しかも重くなるまで誰も気づけない）。
	//
	// これらには limit/offset を任意引数として足したが、**既定値は入れていない**。
	// 既定値を入れると「今まで全件返っていたものが黙って切れる」ため、引数を送って
	// いない既存のクライアントの見え方が変わってしまう。代わりに、引数が無いときは
	// この値で頭打ちにする。
	//
	// 500 なのは「実データがここに届かない」かつ「届いても1クエリとして耐えられる」
	// の両方を満たす所。いま最大のコミュニティでも2桁、返信が3桁に乗る投稿も無い
	// ので、これが効くのは想定外に育ったときだけ。つまり見え方は変わらないまま、
	// 青天井だけが無くなる。
	//
	// ここに当たるようになったら、それはもうページングを入れるべき合図。上限を
	// 上げるのではなく、クライアント側の読み込み UI ごと limit/offset へ移すこと。
	unpagedCollectionCap = 500

	// maxOffset はページ送りで飛べる最大の位置。
	//
	// limit には上限があったが offset には無く、負数を 0 に丸めるだけだった。
	// MySQL の OFFSET は「読み飛ばす行も読んでから捨てる」ので、費用は offset に
	// 比例する。つまり limit を1件に絞っても offset を大きくすれば重くできる
	// （`posts(limit: 1, offset: 10000000)`）。しかも返す件数は1件なので、
	// クエリの複雑度にも現れない。複雑度は「返す量」を測る仕組みで、
	// 「読み飛ばす量」は測れないため、ここで別に止める必要がある。
	//
	// 10000 なのは「実際の画面が到達しない」かつ「到達しても1クエリとして
	// 耐えられる」の両方を満たす所。1ページ20件なら500ページ目に当たり、
	// そこまで送る画面は無い。深いところまで辿りたい要件が出たら、offset では
	// なくカーソル（直前の ID・時刻を起点にする）へ移すこと。offset を上げても
	// 費用は線形に増え続ける。
	maxOffset = 10000
)

// resolveUnpagedWindow は「ページングを後付けしたコレクション」の窓を決める。
//
// limit を送っていなければ unpagedCollectionCap（＝実質これまでどおり全件）、
// 送っていれば他の一覧と同じく maxPageSize で頭打ち。offset の負数丸めも共通。
//
// 既定値をスキーマに書かずにここで面倒を見ているのは、スキーマに既定値を書くと
// 引数を送っていないクライアントまで切れてしまうため（unpagedCollectionCap の
// コメント参照）。
func resolveUnpagedWindow(limit *int32, offset *int32) repository.PageQuery {
	o := resolveOffset(offset)
	l := unpagedCollectionCap
	if limit != nil && *limit > 0 {
		l = resolveLimit(limit, defaultPageSize, maxPageSize)
	}
	return repository.PageQuery{Limit: l, Offset: o}
}

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
	return resolveLimit(limit, fallback, maxPageSize), resolveOffset(offset)
}

// resolveOffset は offset を「負数は 0、上限超は maxOffset」に正規化する。
//
// 頭打ちにして拒否しないのは、limit と揃えるため（limit も上限超は黙って
// 丸める）。ここだけエラーにすると、深いページへ飛んだクライアントが
// 「リストの末尾」ではなく失敗を受け取ることになる。
func resolveOffset(offset *int32) int {
	if offset == nil || *offset <= 0 {
		return 0
	}
	o := int(*offset)
	if o > maxOffset {
		return maxOffset
	}
	return o
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

func resolvePageQuery(ctx context.Context, limit *int32, offset *int32, fallback int) repository.PageQuery {
	l, o := resolvePagination(limit, offset, fallback)
	return repository.PageQuery{Limit: l, Offset: o, WithTotal: fieldRequested(ctx, "total")}
}
