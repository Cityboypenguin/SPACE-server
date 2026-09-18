package repository

import "context"

// SSETicketRepository は SSE 接続専用の短命チケットを預かる。
//
// なぜチケットが要るか: ブラウザ標準の EventSource はカスタムヘッダーを送れないため、
// /events の認証情報は URL かクッキーでしか運べない。JWT を URL に載せると
// アクセスログ・プロキシ・監視基盤に**アクセストークンそのもの**が残る（有効期限まで
// 再利用できてしまう）。そこで「1回使ったら消える・数十秒で失効する」引換券だけを
// URL に載せる。ログに残っても、拾った時点では既に使用済みか期限切れになっている。
//
// Consume が「取得と削除を不可分に」行うことが肝。分けて実装すると、同じチケットで
// 同時に2本張られる隙間ができる。
type SSETicketRepository interface {
	// Issue はチケットを userID に紐づけて保存する。TTL は実装側が持つ
	// （利用側がうっかり長い寿命を渡せないようにするため）。
	Issue(ctx context.Context, ticket string, userID int64) error

	// Consume はチケットを引き換える。存在すれば userID と true を返し、同時に
	// チケットを消す（＝2回目以降は false）。存在しない・期限切れなら false。
	Consume(ctx context.Context, ticket string) (int64, bool, error)
}
