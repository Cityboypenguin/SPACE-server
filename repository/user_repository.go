package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// UserRepository は「公開情報の取得」と「認証情報を含む取得」を分けてある。
//
// 分ける前は GetUserByID も ListUsers も ListUsersByRoomIDs も、表示にしか使わない
// のに hashed_password まで SELECT していた。秘密が載る経路が増えるほど、
// 取り違えて外へ出す事故の可能性も増える。
//
// 規則は2つ:
//   - 表示系（この下の「公開情報」区画）は hashed_password を SELECT しない。
//     戻り値の model.User はそもそもハッシュを持てないので、うっかり載せることも
//     できない（model.User のコメント参照）。
//   - 認証情報が要る経路だけが「認証情報」区画を呼ぶ。呼んでよいのはログイン・
//     パスワード変更・パスワード再設定・新規登録だけ。
//
// トークン検証（internal/auth.ValidateAndVerifyToken）は凍結状態しか見ないので
// 公開情報の GetUserByID を使う。DataLoader の UserLoader も表示用途なので
// GetUsersByIDs（公開情報）を使う。
type UserRepository interface {
	// --- 公開情報 -----------------------------------------------------------
	// いずれも hashed_password を SELECT しない。

	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	GetUsersByIDs(ctx context.Context, ids []int64) ([]*model.User, error)
	// FindByEmail は「そのメールが登録済みか」を見るためのもの（OTP 送信・
	// パスワード再発行の入口）。照合に使うハッシュは返さないので、ログインは
	// FindCredentialsByEmail を使うこと。
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	// ListUsers / SearchUsersByKeyword は PageQuery.WithTotal が true のときだけ
	// COUNT を撃つ。false なら total は 0（PageQuery のコメント参照）。
	ListUsers(ctx context.Context, q PageQuery) ([]*model.User, int, error)
	SearchUsersByKeyword(ctx context.Context, keyword string, q PageQuery) ([]*model.User, int, error)
	// GetUsersByAccountIDs は accountID からユーザーをまとめて引く（メンション解決用）。
	// 照合は DB の照合順序に従うため、大文字小文字は区別しない。
	GetUsersByAccountIDs(ctx context.Context, accountIDs []string) ([]*model.User, error)
	// SuggestUsersByPrefix は accountID が prefix に前方一致するユーザーを返す（メンションのサジェスト用）。
	SuggestUsersByPrefix(ctx context.Context, prefix string, limit int) ([]*model.User, error)
	// UpdateUser は公開列だけを UPDATE する。hashed_password には触れない。
	//
	// 触れないことが大事。以前は *model.User を受けて hashed_password = ? も
	// 書いており、公開情報しか読んでいない経路（凍結・解凍・プロフィール更新）が
	// そのまま保存するとハッシュが空文字で上書きされる形になっていた。
	UpdateUser(ctx context.Context, u *model.User) error
	DeleteUser(ctx context.Context, id int64) (bool, error)
	UpdateLastActiveAt(ctx context.Context, userID int64, now int64) error
	LogActivityDate(ctx context.Context, userID int64, jstDate string) error

	// --- 認証情報 -----------------------------------------------------------
	// hashed_password を読む／書く。呼んでよい経路は UserRepository のコメント参照。

	// FindCredentialsByEmail はログイン用。存在しなければ (nil, nil)。
	FindCredentialsByEmail(ctx context.Context, email string) (*model.UserCredentials, error)
	// GetCredentialsByID はパスワード変更時の現在パスワード照合用。
	// 存在しなければ (nil, nil)。
	GetCredentialsByID(ctx context.Context, id int64) (*model.UserCredentials, error)
	// SaveCredentials は新規登録・パスワード変更の保存。ID が 0 なら INSERT、
	// それ以外は hashed_password を含めた UPDATE。
	SaveCredentials(ctx context.Context, c *model.UserCredentials) error
}
