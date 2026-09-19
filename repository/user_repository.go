package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// UserRepository は取得を「公開情報」「本人・管理者向け」「認証情報」の3段に分けてある。
//
// 分ける前は GetUserByID も ListUsers も ListUsersByRoomIDs も、表示にしか使わない
// のに hashed_password まで SELECT していた。秘密が載る経路が増えるほど、
// 取り違えて外へ出す事故の可能性も増える。メールアドレスも同じで、以前は表示系の
// 取得すべてが他人の連絡先を一緒に持ち上げていた（そして GraphQL の User.email 経由で
// 実際に外へ出ていた）。
//
// 規則は3つ:
//   - 表示系（「公開情報」区画）は email も hashed_password も SELECT しない。
//     戻り値の model.User はそもそもどちらも持てないので、うっかり載せることも
//     できない（model.User のコメント参照）。
//   - 連絡先が要る経路だけが「本人・管理者向け」区画を呼ぶ。呼んでよいのは
//     「本人だけ」か「管理者だけ」が辿れるとリゾルバで保証できる口
//     （me / users / getUserByID / adminSearchUsers / adminUpdateUser / updateUser）。
//   - 認証情報が要る経路だけが「認証情報」区画を呼ぶ。呼んでよいのはログイン・
//     パスワード変更・パスワード再設定・新規登録だけ。
//
// トークン検証（internal/auth.ValidateAndVerifyToken）は凍結状態しか見ないので
// 公開情報の GetUserByID を使う。DataLoader の UserLoader も表示用途なので
// GetUsersByIDs（公開情報）を使う。
type UserRepository interface {
	// --- 公開情報 -----------------------------------------------------------
	// いずれも email も hashed_password も SELECT しない。

	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	GetUsersByIDs(ctx context.Context, ids []int64) ([]*model.User, error)
	// FindByEmail は「そのメールが登録済みか」を見るためのもの（OTP 送信・
	// パスワード再発行の入口）。照合に使うハッシュは返さないので、ログインは
	// FindCredentialsByEmail を使うこと。
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	// SearchUsersByKeyword は PageQuery.WithTotal が true のときだけ
	// COUNT を撃つ。false なら total は 0（PageQuery のコメント参照）。
	// 一般ユーザーの検索なので連絡先は返さない。管理画面の検索は
	// SearchUserAccountsByKeyword を使うこと。
	SearchUsersByKeyword(ctx context.Context, keyword string, q PageQuery) ([]*model.User, int, error)
	// GetUsersByAccountIDs は accountID からユーザーをまとめて引く（メンション解決用）。
	// 照合は DB の照合順序に従うため、大文字小文字は区別しない。
	GetUsersByAccountIDs(ctx context.Context, accountIDs []string) ([]*model.User, error)
	// SuggestUsersByPrefix は accountID が prefix に前方一致するユーザーを返す（メンションのサジェスト用）。
	SuggestUsersByPrefix(ctx context.Context, prefix string, limit int) ([]*model.User, error)
	// UpdateUser は公開列だけを UPDATE する。email にも hashed_password にも触れない。
	//
	// 触れないことが大事。以前は *model.User を受けて hashed_password = ? も
	// 書いており、公開情報しか読んでいない経路（凍結・解凍・プロフィール更新）が
	// そのまま保存するとハッシュが空文字で上書きされる形になっていた。
	// email も同じ理屈で SET から外してある（model.User が Email を持たなくなった
	// 以上、ここで書こうとすれば空文字になる）。連絡先の変更は SaveCredentials 経由。
	UpdateUser(ctx context.Context, u *model.User) error
	DeleteUser(ctx context.Context, id int64) (bool, error)
	DeleteActivityHistory(ctx context.Context, userID int64) error
	UpdateLastActiveAt(ctx context.Context, userID int64, now int64) error
	// LogActivityDate は活動日（JST の "2006-01-02"）を1行残す。日次の
	// activeUsers はこの履歴から数える。
	LogActivityDate(ctx context.Context, userID int64, jstDate string) error
	// LogActivityHour は活動した時間帯（JST の "2006-01-02 15:00:00"）を1行残す。
	// 時間別の activeUsers はこの履歴から数える。last_active_at は1ユーザー1値
	// なので「10時にも14時にも活動した」を表せず、時間別の集計には使えない。
	//
	// LogActivityDate と両方書くのは移行中だからで、この表の履歴が日次グラフの
	// 範囲を覆えば日次もここから導出して user_activity_dates を落とせる
	// （db/migrations/070_create_user_activity_hours.up.sql）。
	LogActivityHour(ctx context.Context, userID int64, jstHour string) error

	// --- 本人・管理者向け ---------------------------------------------------
	// email を SELECT する。hashed_password は読まない。
	// 呼んでよい経路は UserRepository のコメント参照。

	// GetUserAccountByID は本人（me）と管理者（getUserByID）の1件取得。
	// 表示のためにユーザーを引くだけなら GetUserByID を使うこと。
	GetUserAccountByID(ctx context.Context, id int64) (*model.UserAccount, error)
	// GetUserAccountsByIDs はまとめて引く版。規約の同意者一覧（管理者専用）が
	// 「同意レコードの user_id を人に解決する」ために使う。
	// 表示用途は GetUsersByIDs（DataLoader 経由）を使うこと。
	GetUserAccountsByIDs(ctx context.Context, ids []int64) ([]*model.UserAccount, error)
	// ListUserAccounts / SearchUserAccountsByKeyword は管理画面のユーザー台帳用。
	// PageQuery.WithTotal が true のときだけ COUNT を撃つ。
	ListUserAccounts(ctx context.Context, q PageQuery) ([]*model.UserAccount, int, error)
	SearchUserAccountsByKeyword(ctx context.Context, keyword string, q PageQuery) ([]*model.UserAccount, int, error)

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
