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
// 使う限りこの競合は消せない。messages.id は AUTO_INCREMENT で単調増加するので、
// 同じ秒に並んだ行にも必ず前後がつき、m.id > last_read_message_id で切れる。
//
// ID が保証するのは「採番順」であって「コミット順」ではない（未解決・影響は限定的）:
// AUTO_INCREMENT はトランザクションが INSERT を実行した瞬間に値を配るだけで、
// その値の順にコミットされることは保証しない。したがって次の並びが成立しうる。
//
//  1. 送信トランザクション A が id=100 を採番する（まだコミットしていない。
//     メッセージ送信は本文とメディア行を1つのトランザクションで書くので、
//     採番からコミットまでに実時間の幅がある）。
//  2. 後から始まった送信トランザクション B が id=101 を採番し、先にコミットする。
//  3. その瞬間に誰かが既読を打つ。既読位置には「見えている最大 ID」である 101 が入る。
//  4. その後 A がコミットする。id=100 は 100 <= 101 なので未読条件
//     (m.id > last_read_message_id) を満たさず、以後ずっと未読に数えられない。
//
// 3つの条件（A が未コミット・B が先にコミット・その隙に既読）が同時に起きたときだけ
// なので頻度は低いが、窓は現実に存在する。
//
// 影響は未読バッジの取りこぼしだけに限られる。メッセージ自体は保存済みで、購読
// イベント（messageAdded）と通常のページング（before/after カーソル）では必ず出る。
// 消えるのは「未読として数えられる権利」であって、メッセージが見えなくなるわけではない。
//
// 厳密に直すなら（今回はやらない。必要になった人へ）:
//   - ルーム単位のコミット順シーケンスを持つ（コミット直前に採番して保存する連番。
//     採番の競合をルームごとに直列化する必要があるぶん、送信のスループットを削る）。
//   - 読み取りスナップショットに対応したカーソル（未コミットの穴を「まだ確定して
//     いない」と表現できる形。例: PostgreSQL の txid_snapshot 相当の情報を持つ）。
//   - 明示的な read receipt（「このIDまで読んだ」を利用者ごとに集合で持ち、
//     watermark ひとつに畳まない）。
//
// いずれも既読位置の持ち方そのものを変えるので、未読を数える全経路
// （infra/mysql/unread_origin.go を通る6本）に波及する。
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
