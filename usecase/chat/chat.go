// Package chat はチャット（授業内チャット・コミュニティ・DM）のアプリケーション
// サービス。チャットの業務ルールをここ1箇所に集める。
//
// # 4つのサービスに分かれている
//
// 以前は Service ひとつが認可・メッセージ・既読・イベントまで全部を持ち、依存が
// 20個近い「小さなリゾルバ」になっていた。責務ごとに分け、それぞれの依存を
// 自分が実際に使うものだけに絞ってある。
//
//	AccessPolicy          … 誰がどのルームを読める / 書ける / 直せるか（access.go）
//	MessageCommandService … 送信・編集・削除（command.go）
//	MessageQueryService   … メッセージ一覧の取得（query.go）
//	ReadReceiptService    … 既読の記録と未読件数（read.go）
//
// 後ろの3つは AccessPolicy を依存として受け取る。判定の実体を1つに保つためで、
// 各サービスが自前で membership やブロックを見に行く形にしてはいけない
// （統合前はそれで「一覧は出るが購読は繋がらない」「退出後も編集できる」と
// いった食い違いが生まれていた）。
//
// 全部入りのファサードはあえて作っていない。作ると呼び出し側から見える姿が
// 分割前と同じになり、依存も再び1箇所へ集まるため。GraphQL リゾルバは
// graph.ChatUseCases として4つを個別に持つ。
//
// # 保存処理との関係
//
// 実際の保存（INSERT/UPDATE）は usecase/chat/internal/messagestore にある。
// internal に置いてあるので、認可を通さずに保存処理へ辿り着く経路は
// コンパイラが塞ぐ（詳細はそのパッケージのコメント）。配線から保存処理を
// 渡す唯一の口が MessageWriters（writers.go）で、中身は非公開なので
// 束ごと MessageCommandService へ渡すことしかできない。
//
// リゾルバに残る仕事は「GraphQL ID のデコード」と「GraphQL 型への変換」だけ。
package chat

import (
	"github.com/rs/zerolog"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

// logChatCommand は「取れなくても処理は続ける／安全側に倒す」判断の材料が
// 引けなかったことを、必ず同じ形で残す。
//
// 配信側の logChatDelivery（graph/chat_events.go）と同じ流儀で、component で
// 絞れば取りこぼしが一望できる。黙って捨てると DB 障害が「通知が来ない」
// 「権限がないと言われる」という正常な拒否の顔をして紛れてしまう。
// 呼び出し側は room_id / message_id を足してから Msg すること。
func logChatCommand(err error) *zerolog.Event {
	return logger.Log.Error().Err(err).
		Str("component", "chat_command")
}

// containsInt64 は membership 判定用の小さなヘルパー。
// 対象は1ルームのメンバー一覧なので、線形探索で十分。
func containsInt64(values []int64, target int64) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// soleOtherMember は memberIDs の中の「selfID 以外がちょうど1人」のときだけ、
// その1人と true を返す。DM の相手を決めるためのもの。
//
// 「自分以外の最初の1人」ではなく「ちょうど1人」を条件にしているのは、相手が
// 決まらない状態（相手の退会で room_users の行が消えて0人、データ不整合で2人以上）を
// 呼び出し側が明示的に扱えるようにするため。ここで先頭を拾って返すと、相手が特定
// できていないのに特定できたことにして判定を進めてしまう。
func soleOtherMember(memberIDs []int64, selfID int64) (int64, bool) {
	var partnerID int64
	found := 0
	for _, id := range memberIDs {
		if id == selfID {
			continue
		}
		partnerID = id
		found++
	}
	if found != 1 {
		return 0, false
	}
	return partnerID, true
}
