package mysql

import (
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 未読判定「どこから先を未読と数えるか」の唯一の実装。
//
// 規則そのもの（ID 優先 → 時刻 → フォールバック）と、なぜ ID が正なのかは
// repository.UnreadOrigin / repository.ReadPosition のコメントに書いてある。
// ここに集約しているのは、未読を数える経路が6本（1ルーム / ルームID群 /
// ルーム種別 / メンバーごと / 履修者ごと / 授業一覧）あり、条件式を各クエリへ
// 写経すると片方だけ直し忘れて画面ごとに数が食い違うため。
// 新しいカウント経路を足すときも、必ずこの2つの関数を通すこと。

// unreadAfterSQL は既読位置の行を JOIN 済みのクエリ向けに条件式を組み立てる。
//
//	msgAlias        … messages の別名（例 "m"）
//	readAlias       … 既読位置の行の別名（room_users なら "ru"、course_room_reads なら "cr"）
//	fallbackAtExpr  … 既読位置がまったく無いときの起点を表す SQL 式。
//	                  通常ルームは "NULL"（＝全件未読）、授業ルームは時間割の登録時刻
//	                  （例 "t.created_at"）。
//
// readAlias の行が LEFT JOIN で見つからなかった場合は両方の列が NULL になるので、
// 自然に3段目（フォールバック）へ落ちる。
func unreadAfterSQL(msgAlias, readAlias, fallbackAtExpr string) string {
	return fmt.Sprintf(`(CASE
			WHEN %[2]s.last_read_message_id IS NOT NULL THEN %[1]s.id > %[2]s.last_read_message_id
			WHEN %[2]s.last_read_at IS NOT NULL THEN %[1]s.created_at > %[2]s.last_read_at
			ELSE %[1]s.created_at > COALESCE(%[3]s, 0)
		END)`, msgAlias, readAlias, fallbackAtExpr)
}

// unreadAfterCondition は同じ規則を、既読位置を Go 側で解決済みのクエリ
// （1ルームぶんを数える経路）向けに条件式とプレースホルダの値で返す。
func unreadAfterCondition(msgAlias string, origin repository.UnreadOrigin) (string, interface{}) {
	if origin.LastReadMessageID != nil {
		return msgAlias + ".id > ?", *origin.LastReadMessageID
	}
	if origin.LastReadAt != nil {
		return msgAlias + ".created_at > ?", *origin.LastReadAt
	}
	var fallbackAt int64
	if origin.FallbackAt != nil {
		fallbackAt = *origin.FallbackAt
	}
	return msgAlias + ".created_at > ?", fallbackAt
}

// advanceLastReadMessageIDSQL は既読位置の列を「進める方向にだけ」更新する式。
//
// 引数の ? には新しい位置（ルームの最新メッセージID）が入る。値が NULL になるのは
// メッセージが1件も無いルームを既読にしたときで、そのときは既存の位置を保つ。
// NULLIF(..., 0) は「どちらも位置を持っていない」場合に 0 ではなく NULL を書き戻す
// ためのもの（0 を書くと「既読位置あり」と誤って解釈され、授業ルームの
// 時間割登録時刻フォールバックが効かなくなる）。
func advanceLastReadMessageIDSQL(column string) string {
	return fmt.Sprintf("NULLIF(GREATEST(COALESCE(%[1]s, 0), COALESCE(?, 0)), 0)", column)
}
