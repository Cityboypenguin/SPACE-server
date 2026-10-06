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
//
// 利用者は2つの表にまたがる（db/migrations/076）。users は識別子と退会の段階だけを
// 持ち、名前・メールアドレス・パスワードなどの個人情報は user_accounts にある。
// 列には必ず表の別名（u / a）を付け、FROM は userFrom で揃える。
// 完全削除した利用者は user_accounts の行が無いので、この JOIN では一切出てこない。

// userFrom は利用者を読むときの FROM 句。
const userFrom = `users u JOIN user_accounts a ON a.user_id = u.id`

// visibleUserCond は「周りから見える利用者」の条件。退会手続き中の人を除く。
//
// 退会手続き中の人は、猶予の間はログインすれば戻れるので個人情報を残してあるが、
// 周りからは退会済みと同じに見せる。表示系の取得がこの人を返さなければ、
// 投稿者は「削除されたアカウント」になり、トークンの検証（GetUserByID）も通らない。
// 本人・管理者向けと認証情報の取得には付けない（管理画面に出し、ログインで取り消せるように）。
const visibleUserCond = `u.status <> '` + model.UserStatusDeactivated + `'`

// userPublicColumns は表示に使う列。email も hashed_password も含めないこと。
//
// email が抜けているのは、他人の連絡先を表示系の SELECT に載せないため。
// 以前はここに email があり、投稿の作者・ルームのメンバー・検索結果を引くたびに
// 他人のメールアドレスを DB から持ち上げていた（GraphQL の User.email 経由で
// 実際に外へ出てもいた）。hashed_password と同じ扱いにしてある: 要らない経路では
// そもそも引かない。
const userPublicColumns = `u.id, a.account_id, a.name, a.role, u.status, u.created_at, a.updated_at`

// userAccountColumns は本人・管理者向けの列。表示用の列に email を足しただけ。
//
// email を末尾に足しているのは、scanUser（公開列）の並びをそのまま流用して
// 最後に1つ読み足せるようにするため。公開列を並べ替えたら両方の scan が壊れるので、
// 列数と並びは user_projection_test.go で固定してある。
const userAccountColumns = userPublicColumns + `, a.email`

// userCredentialColumns は認証に使う列。本人・管理者向けの列に hashed_password を足しただけ。
const userCredentialColumns = userAccountColumns + `, a.hashed_password, a.credentials_version`

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
		&c.CredentialsVersion,
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
		`SELECT `+userPublicColumns+` FROM `+userFrom+` WHERE u.id = ? AND `+visibleUserCond, id)

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
		fmt.Sprintf(`SELECT `+userPublicColumns+` FROM `+userFrom+` WHERE u.id IN (%s) AND `+visibleUserCond, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUsers(rows)
}

// --- 退会の段階 -------------------------------------------------------------
// 段階の遷移は repository.UserLifecycleRepository のコメントを参照。

func (r *MySQLUserRepository) DeactivateUser(ctx context.Context, id int64, at time.Time) (bool, error) {
	// 認証情報の世代も同じ文で進める。発行済みのアクセストークン・リフレッシュ
	// トークンは世代を見て失効するので、退会した瞬間に他の端末のログインも切れる。
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE users u
		JOIN user_accounts a ON a.user_id = u.id
		SET u.status = ?, u.deactivated_at = ?, a.credentials_version = a.credentials_version + 1
		WHERE u.id = ? AND u.status = ?
	`, model.UserStatusDeactivated, at.Unix(), id, model.UserStatusActive)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *MySQLUserRepository) ReactivateUser(ctx context.Context, id int64) (bool, error) {
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE users SET status = ?, deactivated_at = NULL
		WHERE id = ? AND status = ?
	`, model.UserStatusActive, id, model.UserStatusDeactivated)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *MySQLUserRepository) PurgeUser(ctx context.Context, id int64, at time.Time) (bool, error) {
	db := extractDB(ctx, r.DB)
	// 個人情報の行を消すと、本人の持ち物は外部キーの CASCADE で一緒に消える
	// （どの表がそうなっているかは db/migrations/086〜101）。会話（メッセージ・
	// 質問・回答・投票）は users を参照しているので残る。
	result, err := db.ExecContext(ctx, `DELETE FROM user_accounts WHERE user_id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE users SET status = ?, deleted_at = ? WHERE id = ?`,
		model.UserStatusDeleted, at.Unix(), id,
	); err != nil {
		return false, err
	}
	return true, nil
}

func (r *MySQLUserRepository) LockUserLifecycle(ctx context.Context, id int64) (*model.UserLifecycle, error) {
	var l model.UserLifecycle
	var deactivatedAt sql.NullInt64
	err := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT status, deactivated_at FROM users WHERE id = ? FOR UPDATE`, id,
	).Scan(&l.Status, &deactivatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if deactivatedAt.Valid {
		at := time.Unix(deactivatedAt.Int64, 0)
		l.DeactivatedAt = &at
	}
	return &l, nil
}

func (r *MySQLUserRepository) ListUserIDsToPurge(ctx context.Context, before time.Time, limit int) ([]int64, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, `
		SELECT id FROM users
		WHERE status = ? AND deactivated_at <= ?
		ORDER BY deactivated_at ASC, id ASC
		LIMIT ?
	`, model.UserStatusDeactivated, before.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *MySQLUserRepository) DeleteActivityHistory(ctx context.Context, userID int64) error {
	db := extractDB(ctx, r.DB)
	if _, err := db.ExecContext(ctx, `DELETE FROM user_activity_dates WHERE user_id = ?`, userID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `DELETE FROM user_activity_hours WHERE user_id = ?`, userID)
	return err
}

// --- 本人・管理者向け -------------------------------------------------------
// ここだけが email を SELECT する（hashed_password は読まない）。

// GetUserAccountByID は本人・管理者向けの1件取得。表示のために引くだけなら
// GetUserByID（連絡先なし）を使うこと。
func (r *MySQLUserRepository) GetUserAccountByID(ctx context.Context, id int64) (*model.UserAccount, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT `+userAccountColumns+` FROM `+userFrom+` WHERE u.id = ?`, id)

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
		fmt.Sprintf(`SELECT `+userAccountColumns+` FROM `+userFrom+` WHERE u.id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUserAccounts(rows)
}

// ListUserAccounts は管理画面のユーザー台帳。一覧に連絡先が要るのは管理者だけなので、
// 公開の一覧は用意していない（必要になったら ListUsers を足すこと）。
func (r *MySQLUserRepository) ListUserAccounts(ctx context.Context, q repository.PageQuery) ([]*model.UserAccount, int, error) {
	total, err := countForPage(ctx, r.DB, q, `SELECT COUNT(*) FROM `+userFrom)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT `+userAccountColumns+`
		FROM `+userFrom+`
		ORDER BY u.created_at DESC, u.id DESC
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
		`SELECT COUNT(*) FROM `+userFrom+` WHERE a.name LIKE ? OR a.account_id LIKE ?`,
		searchParam, searchParam,
	)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT `+userAccountColumns+`
		FROM `+userFrom+`
		WHERE a.name LIKE ? OR a.account_id LIKE ?
		ORDER BY a.name ASC, u.id ASC
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
		`SELECT COUNT(*) FROM `+userFrom+` WHERE (a.name LIKE ? OR a.account_id LIKE ?) AND `+visibleUserCond,
		searchParam, searchParam,
	)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT `+userPublicColumns+`
		FROM `+userFrom+`
		WHERE (a.name LIKE ? OR a.account_id LIKE ?) AND `+visibleUserCond+`
		ORDER BY a.name ASC, u.id ASC
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
//
// 退会手続き中の人も返す。メールアドレスは猶予の間その人のもので、新規登録に
// 使わせない（同じアドレスで登録できるのは完全削除の後）。パスワードを忘れた人が
// 再設定して、ログインで退会を取り消す道も残る。
func (r *MySQLUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT `+userPublicColumns+` FROM `+userFrom+` WHERE a.email = ? LIMIT 1`, email)

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
		`SELECT `+userCredentialColumns+` FROM `+userFrom+` WHERE a.email = ? LIMIT 1`, email)

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
		`SELECT `+userCredentialColumns+` FROM `+userFrom+` WHERE u.id = ?`, id)

	c, err := scanUserCredentials(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *MySQLUserRepository) GetCredentialsVersionByID(ctx context.Context, id int64) (int64, error) {
	var version int64
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, `SELECT credentials_version FROM user_accounts WHERE user_id = ?`, id).Scan(&version)
	return version, err
}

// UpdateUser は公開列だけを更新する。hashed_password は SET に入れない。
//
// 入れないことが要。呼び出し元（凍結・解凍・プロフィール更新）は公開情報しか
// 読んでいないので、以前の「u.HashedPassword をそのまま書く」形だと、ハッシュを
// 持たない model.User を保存した瞬間に空文字で上書きしてしまう。
func (r *MySQLUserRepository) UpdateUser(ctx context.Context, u *model.User) error {
	u.UpdatedAt = time.Now()

	_, err := extractDB(ctx, r.DB).ExecContext(ctx, `
		UPDATE users u
		JOIN user_accounts a ON a.user_id = u.id
		SET a.account_id = ?, a.name = ?, a.role = ?, u.status = ?, a.updated_at = ?
		WHERE u.id = ?
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
//
// 個人情報（user_accounts）と状態（users）を別々の UPDATE にしているのは順番のため。
// credentials_version は「書き換える前の hashed_password」と比べて進める必要があり、
// 1文の中で比較が代入より先に評価されなければならない。MySQL は1表の UPDATE なら
// 代入を左から順に評価するが、複数表の UPDATE では順番を保証しない。まとめると、
// パスワードを変えても世代が進まず、古いトークンが生き残りうる。
func (r *MySQLUserRepository) updateCredentials(ctx context.Context, c *model.UserCredentials) error {
	err := inTx(ctx, r.DB, func(ctx context.Context) error {
		db := extractDB(ctx, r.DB)
		if _, err := db.ExecContext(ctx, `
			UPDATE user_accounts
			SET account_id = ?, name = ?, email = ?,
			    credentials_version = credentials_version + IF(hashed_password <> ?, 1, 0),
			    hashed_password = ?, role = ?, updated_at = ?
			WHERE user_id = ?
		`,
			c.AccountID,
			c.Name,
			c.Email,
			c.HashedPassword,
			c.HashedPassword,
			c.Role,
			c.UpdatedAt.Unix(),
			c.ID,
		); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx, `UPDATE users SET status = ? WHERE id = ?`, c.Status, c.ID)
		return err
	})
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

// createUser は識別子（users）と個人情報（user_accounts）の2行を作る。
// 片方だけ残らないよう、呼び出し側がトランザクションを張っていなければ自分で張る。
func (r *MySQLUserRepository) createUser(ctx context.Context, c *model.UserCredentials) (int64, error) {
	var id int64
	err := inTx(ctx, r.DB, func(ctx context.Context) error {
		db := extractDB(ctx, r.DB)
		result, err := db.ExecContext(ctx,
			`INSERT INTO users (status, created_at) VALUES (?, ?)`,
			c.Status, c.CreatedAt.Unix(),
		)
		if err != nil {
			return err
		}
		if id, err = result.LastInsertId(); err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `
			INSERT INTO user_accounts (user_id, account_id, name, email, hashed_password, role, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`,
			id,
			c.AccountID,
			c.Name,
			c.Email,
			c.HashedPassword,
			c.Role,
			c.UpdatedAt.Unix(),
		)
		return err
	})
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
	return id, nil
}

func (r *MySQLUserRepository) UpdateLastActiveAt(ctx context.Context, userID int64, now int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`UPDATE user_accounts SET last_active_at = ? WHERE user_id = ? AND (last_active_at IS NULL OR last_active_at < ?)`,
		now, userID, now-300, // 5分以内の重複更新をスキップ
	)
	return err
}

func (r *MySQLUserRepository) LogActivityDate(ctx context.Context, userID int64, jstDate string) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT IGNORE INTO user_activity_dates (user_id, activity_date) SELECT user_id, ? FROM user_accounts WHERE user_id = ?`,
		jstDate, userID,
	)
	return err
}

// LogActivityHour は「その人がその時間帯に活動した」を1行残す。
//
// 1ユーザー1日あたり最大24行（実際は活動した時間帯の数）で増えるため、
// internal/activityarchive が400日を超えた完了済み月をCSV.gzへ退避する。
// jstHour は JST の時の始まり（"2006-01-02 15:00:00"）。同じ時間帯の2回目以降は
// INSERT IGNORE が主キー重複として捨てるので、呼び出し側は重複を気にしなくてよい
// （書き込みの回数そのものは middleware 側で間引いている）。
func (r *MySQLUserRepository) LogActivityHour(ctx context.Context, userID int64, jstHour string) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT IGNORE INTO user_activity_hours (user_id, activity_hour) SELECT user_id, ? FROM user_accounts WHERE user_id = ?`,
		jstHour, userID,
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
		FROM `+userFrom+`
		WHERE a.account_id IN (%s) AND u.status = ?`, placeholders)

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
		FROM ` + userFrom + `
		WHERE a.account_id LIKE ? ESCAPE '\\' AND u.status = ?`
	args := []interface{}{escapeLikePrefix(prefix) + "%", model.UserStatusActive}

	query, args, err := AppendBlockFilter(ctx, query, args, "u.id")
	if err != nil {
		return nil, err
	}
	query += " ORDER BY a.account_id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanUsers(rows)
}
