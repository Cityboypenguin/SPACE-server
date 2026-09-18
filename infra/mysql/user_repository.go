package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLUserRepository struct {
	DB *sql.DB
}

func NewMySQLUserRepository(db *sql.DB) *MySQLUserRepository {
	return &MySQLUserRepository{DB: db}
}

// 列リストは「読んでよい範囲」の段階そのもの。上から順に広くなる:
//
//	userPublicColumns      表示系。連絡先も秘密も引かない。
//	userAccountColumns     本人・管理者向け。email を足す。
//	userCredentialColumns  認証のみ。hashed_password を足す。
//
// 定数にしているのは、SELECT を書き足す人が並びを写し間違えても、
// user_projection_test.go が「どの段階に何が入っているか」を固定して見張れるようにするため。

// userPublicColumns は表示に使う列。email も hashed_password も含めないこと。
//
// email が抜けているのは、他人の連絡先を表示系の SELECT に載せないため。
// 以前はここに email があり、投稿の作者・ルームのメンバー・検索結果を引くたびに
// 他人のメールアドレスを DB から持ち上げていた（GraphQL の User.email 経由で
// 実際に外へ出てもいた）。hashed_password と同じ扱いにしてある: 要らない経路では
// そもそも引かない。
const userPublicColumns = `id, account_id, name, role, status, created_at, updated_at`

// userAccountColumns は本人・管理者向けの列。表示用の列に email を足しただけ。
//
// email を末尾に足しているのは、scanUser（公開列）の並びをそのまま流用して
// 最後に1つ読み足せるようにするため。公開列を並べ替えたら両方の scan が壊れるので、
// 列数と並びは user_projection_test.go で固定してある。
const userAccountColumns = userPublicColumns + `, email`

// userCredentialColumns は認証に使う列。本人・管理者向けの列に hashed_password を足しただけ。
const userCredentialColumns = userAccountColumns + `, hashed_password`

// scanUser は userPublicColumns の並びで1行を読む。
func scanUser(row rowScanner) (*model.User, error) {
	var u model.User
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(
		&u.ID,
		&u.AccountID,
		&u.Name,
		&u.Role,
		&u.Status,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		return nil, err
	}
	u.CreatedAt = time.Unix(createdAtUnix, 0)
	u.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &u, nil
}

// scanUserAccount は userAccountColumns の並びで1行を読む。
func scanUserAccount(row rowScanner) (*model.UserAccount, error) {
	var a model.UserAccount
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(
		&a.ID,
		&a.AccountID,
		&a.Name,
		&a.Role,
		&a.Status,
		&createdAtUnix,
		&updatedAtUnix,
		&a.Email,
	); err != nil {
		return nil, err
	}
	a.CreatedAt = time.Unix(createdAtUnix, 0)
	a.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &a, nil
}

// scanUserCredentials は userCredentialColumns の並びで1行を読む。
func scanUserCredentials(row rowScanner) (*model.UserCredentials, error) {
	var c model.UserCredentials
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(
		&c.ID,
		&c.AccountID,
		&c.Name,
		&c.Role,
		&c.Status,
		&createdAtUnix,
		&updatedAtUnix,
		&c.Email,
		&c.HashedPassword,
	); err != nil {
		return nil, err
	}
	c.CreatedAt = time.Unix(createdAtUnix, 0)
	c.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &c, nil
}

// scanUsers は userPublicColumns で SELECT した rows を []*model.User に詰め替える。
func scanUsers(rows *sql.Rows) ([]*model.User, error) {
	var users []*model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// scanUserAccounts は userAccountColumns で SELECT した rows を []*model.UserAccount に詰め替える。
func scanUserAccounts(rows *sql.Rows) ([]*model.UserAccount, error) {
	var accounts []*model.UserAccount
	for rows.Next() {
		a, err := scanUserAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// SaveUser は公開情報の保存には使わない。この型に残っているのは
// SaveCredentials の実体だから（下を見よ）。
func (r *MySQLUserRepository) saveCredentials(ctx context.Context, c *model.UserCredentials) error {
	now := time.Now()
	c.UpdatedAt = now

	if c.ID == 0 {
		c.CreatedAt = now
		id, err := r.createUser(ctx, c)
		if err != nil {
			return err
		}
		c.ID = id
		return nil
	}
	return r.updateCredentials(ctx, c)
}

// SaveCredentials は新規登録・パスワード変更の保存口（認証情報）。
func (r *MySQLUserRepository) SaveCredentials(ctx context.Context, c *model.UserCredentials) error {
	return r.saveCredentials(ctx, c)
}

func (r *MySQLUserRepository) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT `+userPublicColumns+` FROM users WHERE id = ?`, id)

	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *MySQLUserRepository) GetUsersByIDs(ctx context.Context, ids []int64) ([]*model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := extractDB(ctx, r.DB).QueryContext(ctx,
		fmt.Sprintf(`SELECT `+userPublicColumns+` FROM users WHERE id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUsers(rows)
}

func (r *MySQLUserRepository) DeleteUser(ctx context.Context, id int64) (bool, error) {
	query := `DELETE FROM users WHERE id = ?`
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// --- 本人・管理者向け -------------------------------------------------------
// ここだけが email を SELECT する（hashed_password は読まない）。

// GetUserAccountByID は本人・管理者向けの1件取得。表示のために引くだけなら
// GetUserByID（連絡先なし）を使うこと。
func (r *MySQLUserRepository) GetUserAccountByID(ctx context.Context, id int64) (*model.UserAccount, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT `+userAccountColumns+` FROM users WHERE id = ?`, id)

	a, err := scanUserAccount(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// GetUserAccountsByIDs はまとめて引く版。GetUsersByIDs と同じ形で、引く列だけが違う。
func (r *MySQLUserRepository) GetUserAccountsByIDs(ctx context.Context, ids []int64) ([]*model.UserAccount, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := extractDB(ctx, r.DB).QueryContext(ctx,
		fmt.Sprintf(`SELECT `+userAccountColumns+` FROM users WHERE id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUserAccounts(rows)
}

// ListUserAccounts は管理画面のユーザー台帳。一覧に連絡先が要るのは管理者だけなので、
// 公開の一覧は用意していない（必要になったら ListUsers を足すこと）。
func (r *MySQLUserRepository) ListUserAccounts(ctx context.Context, q repository.PageQuery) ([]*model.UserAccount, int, error) {
	total, err := countForPage(ctx, r.DB, q, `SELECT COUNT(*) FROM users`)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT `+userAccountColumns+`
		FROM users
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	accounts, err := scanUserAccounts(rows)
	if err != nil {
		return nil, 0, err
	}
	return accounts, total, nil
}

// SearchUserAccountsByKeyword は管理画面の検索。絞り込み条件も並び順も
// SearchUsersByKeyword と同じで、引く列だけが違う（email を足す）。
// 条件が分かれると「管理画面と一般画面で検索結果が違う」という分かりにくい差になるので、
// 変えるときは両方を揃えること。
func (r *MySQLUserRepository) SearchUserAccountsByKeyword(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.UserAccount, int, error) {
	searchParam := "%" + keyword + "%"

	total, err := countForPage(ctx, r.DB, q,
		`SELECT COUNT(DISTINCT id) FROM users WHERE name LIKE ? OR account_id LIKE ?`,
		searchParam, searchParam,
	)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT DISTINCT `+userAccountColumns+`
		FROM users
		WHERE name LIKE ? OR account_id LIKE ?
		ORDER BY name ASC, id ASC
		LIMIT ? OFFSET ?
	`, searchParam, searchParam, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	accounts, err := scanUserAccounts(rows)
	if err != nil {
		return nil, 0, err
	}
	return accounts, total, nil
}

func (r *MySQLUserRepository) SearchUsersByKeyword(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.User, int, error) {
	searchParam := "%" + keyword + "%"

	total, err := countForPage(ctx, r.DB, q,
		`SELECT COUNT(DISTINCT id) FROM users WHERE name LIKE ? OR account_id LIKE ?`,
		searchParam, searchParam,
	)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT DISTINCT `+userPublicColumns+`
		FROM users
		WHERE name LIKE ? OR account_id LIKE ?
		ORDER BY name ASC, id ASC
		LIMIT ? OFFSET ?
	`, searchParam, searchParam, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users, err := scanUsers(rows)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// FindByEmail は「そのメールが登録済みか」を見るための公開情報の取得。
// ログインの照合には使えない（ハッシュを返さない）。FindCredentialsByEmail を使うこと。
func (r *MySQLUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT `+userPublicColumns+` FROM users WHERE email = ? LIMIT 1`, email)

	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// --- 認証情報 ---------------------------------------------------------------
// ここから3つだけが hashed_password を読み書きする。

func (r *MySQLUserRepository) FindCredentialsByEmail(ctx context.Context, email string) (*model.UserCredentials, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT `+userCredentialColumns+` FROM users WHERE email = ? LIMIT 1`, email)

	c, err := scanUserCredentials(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *MySQLUserRepository) GetCredentialsByID(ctx context.Context, id int64) (*model.UserCredentials, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT `+userCredentialColumns+` FROM users WHERE id = ?`, id)

	c, err := scanUserCredentials(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

// UpdateUser は公開列だけを更新する。hashed_password は SET に入れない。
//
// 入れないことが要。呼び出し元（凍結・解凍・プロフィール更新）は公開情報しか
// 読んでいないので、以前の「u.HashedPassword をそのまま書く」形だと、ハッシュを
// 持たない model.User を保存した瞬間に空文字で上書きしてしまう。
func (r *MySQLUserRepository) UpdateUser(ctx context.Context, u *model.User) error {
	u.UpdatedAt = time.Now()

	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE users
		SET account_id = ?, name = ?, role = ?, status = ?, updated_at = ?
		WHERE id = ?
	`,
		u.AccountID,
		u.Name,
		u.Role,
		u.Status,
		u.UpdatedAt.Unix(),
		u.ID,
	)
	return translateUserWriteError(err)
}

// updateCredentials は公開列に加えて hashed_password も更新する（認証情報）。
func (r *MySQLUserRepository) updateCredentials(ctx context.Context, c *model.UserCredentials) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE users
		SET account_id = ?, name = ?, email = ?, hashed_password = ?, role = ?, status = ?, updated_at = ?
		WHERE id = ?
	`,
		c.AccountID,
		c.Name,
		c.Email,
		c.HashedPassword,
		c.Role,
		c.Status,
		c.UpdatedAt.Unix(),
		c.ID,
	)
	return translateUserWriteError(err)
}

// translateUserWriteError は users への書き込みの一意制約違反を、
// 公開してよい文言へ揃える。
//
// 一意制約違反の判定は isDuplicateKeyError に集約してある（errno 1062）。
// どの列で衝突したかはドライバのメッセージにしか出ないので、ここだけは
// 文字列で列名を見ている。
func translateUserWriteError(err error) error {
	if err == nil {
		return nil
	}
	if isDuplicateKeyError(err) {
		if strings.Contains(err.Error(), "account_id") {
			return errors.New("account_id is already taken")
		}
		return errors.New("email update failed")
	}
	return err
}

func (r *MySQLUserRepository) createUser(ctx context.Context, c *model.UserCredentials) (int64, error) {
	query := `
		INSERT INTO users (account_id, name, email, hashed_password, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := extractDB(ctx, r.DB).ExecContext(ctx, query,
		c.AccountID,
		c.Name,
		c.Email,
		c.HashedPassword,
		c.Role,
		c.Status,
		c.CreatedAt.Unix(),
		c.UpdatedAt.Unix(),
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			if strings.Contains(err.Error(), "account_id") {
				return 0, errors.New("account_id is already taken")
			}
			// メールアドレス重複は詳細を開示しない（ユーザー列挙攻撃対策）
			return 0, errors.New("registration failed: check your input")
		}
		return 0, err
	}
	return result.LastInsertId()
}

func (r *MySQLUserRepository) UpdateLastActiveAt(ctx context.Context, userID int64, now int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`UPDATE users SET last_active_at = ? WHERE id = ? AND (last_active_at IS NULL OR last_active_at < ?)`,
		now, userID, now-300, // 5分以内の重複更新をスキップ
	)
	return err
}

func (r *MySQLUserRepository) LogActivityDate(ctx context.Context, userID int64, jstDate string) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT IGNORE INTO user_activity_dates (user_id, activity_date) VALUES (?, ?)`,
		userID, jstDate,
	)
	return err
}

// LogActivityHour は「その人がその時間帯に活動した」を1行残す。
//
// この表は消す仕組みが無く、1ユーザー1日あたり最大24行（実際は活動した時間帯の数）で
// 単調増加する。保持期間は運用要件なのでここでは決めていない。見積もりと申し送りは
// db/migrations/070_create_user_activity_hours.up.sql のコメントに1箇所だけ書いてある。
// jstHour は JST の時の始まり（"2006-01-02 15:00:00"）。同じ時間帯の2回目以降は
// INSERT IGNORE が主キー重複として捨てるので、呼び出し側は重複を気にしなくてよい
// （書き込みの回数そのものは middleware 側で間引いている）。
func (r *MySQLUserRepository) LogActivityHour(ctx context.Context, userID int64, jstHour string) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT IGNORE INTO user_activity_hours (user_id, activity_hour) VALUES (?, ?)`,
		userID, jstHour,
	)
	return err
}

// GetUsersByAccountIDs は accountID からユーザーをまとめて引く（メンション解決用）。
// 比較は DB の照合順序（大文字小文字を区別しない）に従うため、
// 本文に "@Taro" と書かれていても accountID が "taro" のユーザーに解決される。
// 凍結ユーザーはメンション先にできないため除外する。
func (r *MySQLUserRepository) GetUsersByAccountIDs(ctx context.Context, accountIDs []string) ([]*model.User, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(accountIDs)), ",")
	query := fmt.Sprintf(`
		SELECT `+userPublicColumns+`
		FROM users
		WHERE account_id IN (%s) AND status = ?`, placeholders)

	args := make([]any, 0, len(accountIDs)+1)
	for _, accountID := range accountIDs {
		args = append(args, accountID)
	}
	args = append(args, model.UserStatusActive)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUsers(rows)
}

// SuggestUsersByPrefix は accountID が prefix に前方一致するユーザーを返す（メンションのサジェスト用）。
// 候補の並びは accountID 昇順で安定させる。ブロック関係にある相手は除外する。
func (r *MySQLUserRepository) SuggestUsersByPrefix(ctx context.Context, prefix string, limit int) ([]*model.User, error) {
	query := `
		SELECT ` + userPublicColumns + `
		FROM users
		WHERE account_id LIKE ? ESCAPE '\\' AND status = ?`
	args := []interface{}{escapeLikePrefix(prefix) + "%", model.UserStatusActive}

	query, args, err := AppendBlockFilter(ctx, query, args, "id")
	if err != nil {
		return nil, err
	}
	query += " ORDER BY account_id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUsers(rows)
}
