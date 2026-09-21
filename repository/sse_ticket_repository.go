package repository

import "context"

// StreamSession は SSE 接続1本ぶんの認証の素。
//
// Token を持たせているのは、接続中に「まだ通用するか」を確かめ直すため。
// 失効の判定はトークン文字列で引く（RevokedTokenRepository）ので、
// userID だけを引き換えると、ログアウトしても既存のストリームが残る。
//
// トークンを預ける置き場が増える点は承知のうえ。寿命は数十秒で、鍵は乱数、
// しかも失効リスト（revoked:<token>）が既に同じ置き場でトークン文字列を
// 扱っている。新しく生まれる危険は無い。
type StreamSession struct {
	UserID int64  `json:"userID"`
	Token  string `json:"token"`
}

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
	// Issue はチケットを session に紐づけて保存する。TTL は実装側が持つ
	// （利用側がうっかり長い寿命を渡せないようにするため）。
	Issue(ctx context.Context, ticket string, session StreamSession) error

	// Consume はチケットを引き換える。存在すれば session と true を返し、同時に
	// チケットを消す（＝2回目以降は false）。存在しない・期限切れなら false。
	Consume(ctx context.Context, ticket string) (StreamSession, bool, error)
}
