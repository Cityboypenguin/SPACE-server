package user

import "github.com/Cityboypenguin/SPACE-server/repository"

// 利用者まわりのユースケースは、触る区画（公開情報／連絡先／認証情報／活動記録）が
// それぞれ違う。repository.UserRepository をまるごと渡すと、表示だけの経路にも
// hashed_password を書き換える口が付いてくる。区画の組み合わせごとに名前を付けて、
// 「この処理は何に触りうるか」がシグネチャから読めるようにしてある。

// userDirectoryRepository は人の一覧・検索・取得。公開情報に加えて、本人と管理者に
// だけ見せる連絡先も扱う（どちらを返すかは呼び出し側が権限で決める）。
type userDirectoryRepository interface {
	repository.UserReader
	repository.UserAccountRepository
}

// userStatusRepository は凍結・解凍。状態列だけを読み書きする。
type userStatusRepository interface {
	repository.UserReader
	repository.UserWriter
}

// userDeletionRepository は退会。本人の行と活動履歴を消す。
type userDeletionRepository interface {
	repository.UserWriter
	repository.UserActivityRepository
}

// userSessionRepository はトークンの再発行。連絡先（トークンに載せる）と
// 認証情報の世代（失効の判定）を見る。
type userSessionRepository interface {
	repository.UserAccountRepository
	repository.UserCredentialsRepository
}

// userPasswordRepository はパスワード再設定。ハッシュを書き換え、
// 同時に本人の行も更新する。
type userPasswordRepository interface {
	repository.UserWriter
	repository.UserCredentialsRepository
}

// userProfileWriteRepository は本人によるアカウント更新（表示名・連絡先）。
type userProfileWriteRepository interface {
	repository.UserWriter
	repository.UserCredentialsRepository
}
