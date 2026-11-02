// Package accountpurge は、退会の猶予が切れた利用者の個人情報を毎日消す。
//
// 消す手順そのものは usecase/user.PurgeExpiredAccountsUseCase にある。ここは
// 「いつ走らせるか」だけを持つ（起動時に1回、その後は毎日 JST 4時）。
//
// 複数のサーバーで同時に走っても壊れない。消す直前に利用者の行をロックして
// 状態を確かめ直すので、同じ人を二重に消すことはなく、後から来た側は何もしない。
package accountpurge

import (
	"github.com/Cityboypenguin/SPACE-server/internal/dailyjob"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// runHourJST は日次で走らせる時刻。利用の少ない時間帯にする。
const runHourJST = 4

func New(purge userusecase.PurgeExpiredAccountsUseCase) *dailyjob.Runner {
	return &dailyjob.Runner{
		Name:        "account_purge",
		HourJST:     runHourJST,
		Exec:        purge.Execute,
		DoneMessage: "purged the personal data of users whose grace period expired",
	}
}
