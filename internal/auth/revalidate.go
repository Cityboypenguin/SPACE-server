package auth

import (
	"context"
	"errors"
	"time"
)

// SessionRevalidateInterval は長寿命の接続で認証をやり直す間隔。
//
// HTTP はリクエストのたびに ValidateAndVerifyToken を通る（失効・凍結・退会・
// パスワード変更がその場で効く）。WebSocket と SSE は接続を張るときに1度しか
// 通らないので、何も足さなければ**接続が生きている限り当時の権限で動き続ける**。
// 管理者を消しても、利用者を凍結しても、ログアウトしても、開いたままのタブには
// 新着が流れ続ける。切るには再起動しかない。
//
// const ではなく var なのはテストから短くするため（本番で書き換えてはいけない）。
var SessionRevalidateInterval = time.Minute

// maxUnavailableChecks は「確かめられなかった」を何回まで見送るか。
//
// 見送るのは、一瞬の不調で全接続を切らないため（ErrVerificationUnavailable の
// コメント参照）。無制限に見送ると今度は「Redis が落ちている間は失効が効かない」
// になるので、続いたら切る。SessionRevalidateInterval との積が、確かめられない
// 状態が続いたときに接続が残りうる最長時間になる。
const maxUnavailableChecks = 10

// SessionChecker は「この接続の認証はいまも通用するか」。
// 実体は ValidateAndVerifyToken を束ねたもの。
type SessionChecker func(ctx context.Context) error

// WatchSession は ctx を派生し、認証が通らなくなった時点でそれを打ち切る。
//
// 返した ctx を接続の親として使うこと。WebSocket なら gqlgen がその ctx の終了で
// ソケットを閉じ、SSE ならハンドラのループがそこで抜ける。つまり呼び出し側は
// 「接続の ctx をこれに差し替える」だけでよく、切る手順を各所に書かなくて済む。
//
// 見張りは ctx が終わった時点で必ず止まる（接続が閉じれば goroutine も消える）。
//
// onClosed は切った理由を残すためのもの。nil でもよい。
func WatchSession(ctx context.Context, check SessionChecker, onClosed func(error)) context.Context {
	if check == nil {
		return ctx
	}
	watched, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		ticker := time.NewTicker(SessionRevalidateInterval)
		defer ticker.Stop()

		unavailable := 0
		for {
			select {
			case <-watched.Done():
				return
			case <-ticker.C:
				err := check(watched)
				if err == nil {
					unavailable = 0
					continue
				}
				if errors.Is(err, ErrVerificationUnavailable) {
					unavailable++
					if unavailable < maxUnavailableChecks {
						continue
					}
				}
				if onClosed != nil {
					onClosed(err)
				}
				return
			}
		}
	}()
	return watched
}
