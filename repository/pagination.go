package repository

// PageQuery はオフセットページングの「窓」と「総件数が要るか」を1つにまとめた型。
//
// これを入れる前は、一覧系のリポジトリ／ユースケースが揃って
// (ctx, ..., limit, offset int) を取り、実装は必ず SELECT と COUNT の2本を撃って
// いた。GraphQL 側が total を選んでいなくても COUNT は必ず走るので、items だけ
// 欲しい画面にも全件走査ぶんの費用が乗る。
//
// 「数えるかどうか」を伝えるのに limit, offset のうしろへ bool を足していくと、
// 呼び出し側が ListX(ctx, id, 20, 0, true, false) のような読めない並びになり、
// 一覧ごとに引数の順番も揃わなくなる。窓の指定そのものが1つの関心事なので、
// 引数を増やすのではなく limit/offset ごとこの型に畳んだ。一覧系は例外なく
// この型を受け取ること（ページングを持たない取得は対象外）。
//
// WithTotal が false のとき、実装は COUNT を撃たず total に 0 を返す。GraphQL が
// total を選んでいないときだけ false になるので、0 が応答に出ることはない。
// 逆に「判定できない」ときは必ず true 側（従来どおり数える）へ倒すこと
// （graph.fieldRequested のコメント参照）。
//
// カーソルページング（MessagePage）は総件数の概念を持たないので、この型は使わない。
type PageQuery struct {
	// Limit は取得件数。呼び出し側（graph.resolvePageQuery）で上限クランプ済み。
	Limit int
	// Offset は読み飛ばす件数。負数は呼び出し側で 0 に丸め済み。
	Offset int
	// WithTotal が true のときだけ、実装は総件数を数える。
	WithTotal bool
}
