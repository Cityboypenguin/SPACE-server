package mysql

import (
	"context"
	"database/sql"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MySQLProfileRepository struct {
	DB *sql.DB
}

func NewMySQLProfileRepository(db *sql.DB) *MySQLProfileRepository {
	return &MySQLProfileRepository{DB: db}
}

// GetProfileByUserID は、指定したユーザーIDのプロフィールをデータベースから探してきます
func (r *MySQLProfileRepository) GetProfileByUserID(ctx context.Context, userID string) (*model.Profile, error) {
	query := `
		SELECT user_id, username, bio, grade, image, created_at, updated_at
		FROM profiles
		WHERE user_id = ?
	`
	row := r.DB.QueryRowContext(ctx, query, userID)

	var p model.Profile
	if err := row.Scan(
		&p.UserID,
		&p.Username,
		&p.Bio,
		&p.Grade,
		&p.Image,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // プロフィールがまだ作成されていない場合は空っぽ(nil)を返す
		}
		return nil, err
	}

	return &p, nil
}

// SaveProfile は、プロフィールをデータベースに保存します
func (r *MySQLProfileRepository) SaveProfile(ctx context.Context, p *model.Profile) error {
	// ポイント: 「ON DUPLICATE KEY UPDATE」を使うことで、
	// まだプロフィールが無ければ新規作成(INSERT)し、既に存在していれば上書き(UPDATE)するという
	// 賢い保存処理を1発でやってくれます。
	query := `
		INSERT INTO profiles (user_id, username, bio, grade, image, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		username = VALUES(username),
		bio = VALUES(bio),
		grade = VALUES(grade),
		image = VALUES(image),
		updated_at = VALUES(updated_at)
	`

	_, err := r.DB.ExecContext(ctx, query,
		p.UserID,
		p.Username,
		p.Bio,
		p.Grade,
		p.Image,
		p.CreatedAt,
		p.UpdatedAt,
	)
	return err
}
