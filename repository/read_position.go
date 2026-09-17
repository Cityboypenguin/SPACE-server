package repository

// ReadPosition は「そのルームをどこまで読んだか」。room_users / course_room_reads の
// どちらの表から読んでも同じ形で返す（未読の数え方を2種類に分けないため）。
//
// LastReadMessageID が正の位置で、LastReadAt は表示用（GraphQL の
// roomReadStatus.lastReadAt）と、ID がまだ入っていない行のフォールバック。
//
// なぜ時刻ではなく ID が正なのか:
// 既読位置もメッセージの作成時刻も Unix 秒なので、既読更新と新着が同じ秒に起きると
// 「既読にした後に保存されたメッセージ」でも created_at > last_read_at を満たさず、
// 未読から永久に漏れる。秒の中での前後関係は時刻からは分からないため、時刻を位置に
// 使う限りこの競合は消せない。messages.id は AUTO_INCREMENT で保存順に単調増加する
// ので、m.id > last_read_message_id なら取りこぼしが原理的に起きない。
type ReadPosition struct {
	// LastReadMessageID は最後に読んだメッセージの ID。未リリースの列が入る前の行や、
	// メッセージが1件も無いルームを既読にした場合は nil。
	LastReadMessageID *int64
	// LastReadAt は既読にした時刻（Unix 秒）。
	LastReadAt *int64
}

// UnreadOrigin は「どこから先を未読として数えるか」の起点。
//
// フォールバック規則（この3段はここだけで定義し、各カウント経路はこの型を通す）:
//
//  1. LastReadMessageID があれば m.id > LastReadMessageID
//  2. 無ければ LastReadAt があれば m.created_at > LastReadAt
//     （既読を打つ前に作られた行＝列追加前から既読位置を持っていた人のための互換経路）
//  3. どちらも無ければ m.created_at > COALESCE(FallbackAt, 0)
//     通常ルーム: FallbackAt は nil＝そのルームの他人のメッセージを全件未読にする。
//     授業ルーム: FallbackAt は時間割に登録した時刻。まだ一度も開いていない授業で
//     「登録前の過去ログ全部」が未読にならないようにする現行の規則を維持する。
//
// SQL 側（JOIN してまとめて数える経路）と Go 側（1ルームだけ数える経路）の実装は
// どちらも infra/mysql/unread_origin.go にあり、条件式を各クエリへ写経しない。
type UnreadOrigin struct {
	LastReadMessageID *int64
	LastReadAt        *int64
	// FallbackAt は既読位置が何も無いときの起点。nil なら全件。
	FallbackAt *int64
}

// NewUnreadOrigin は既読位置（未読なら nil）とフォールバック時刻から起点を作る。
func NewUnreadOrigin(pos *ReadPosition, fallbackAt *int64) UnreadOrigin {
	origin := UnreadOrigin{FallbackAt: fallbackAt}
	if pos != nil {
		origin.LastReadMessageID = pos.LastReadMessageID
		origin.LastReadAt = pos.LastReadAt
	}
	return origin
}
