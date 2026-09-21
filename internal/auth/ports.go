package auth

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// AccountVerifier はトークン検証が利用者について確かめることだけの口。
//
// repository.UserRepository をまるごと受け取らないのは、認証の入口が
// 退会・プロフィール更新・パスワード保存まで呼べる必要がまったく無いため。
// 実装（infra/mysql）はそのまま満たす。
type AccountVerifier interface {
	// GetUserByID は公開情報だけを引く（凍結状態の確認に使う）。
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	// GetCredentialsVersionByID はパスワード変更による失効の判定に使う。
	GetCredentialsVersionByID(ctx context.Context, id int64) (int64, error)
}
