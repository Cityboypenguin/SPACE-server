package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.CourseChatImageRepository = &MySQLCourseChatImageRepository{}

type MySQLCourseChatImageRepository struct {
	DB *sql.DB
}

func NewMySQLCourseChatImageRepository(db *sql.DB) repository.CourseChatImageRepository {
	return &MySQLCourseChatImageRepository{DB: db}
}

// 画像の添付先ごとの SELECT。列の並びは3つとも同じにして UNION ALL で束ねる。
// 授業内チャット = rooms.type = 'course'。質問・回答はもともと授業のルームにしか無い。
// 画像だけを対象にする（content_type が image/）。スレッドは親の授業の画像として扱う。
const courseChatImageColumns = `
	md.id, md.storage_key, md.content_type, md.width, md.height, md.created_at,
	md.uploader_user_id, COALESCE(u.name, '退会済みの利用者'), COALESCE(u.account_id, ''),
	COALESCE(pr.id, r.id) AS room_id, COALESCE(pr.name, r.name) AS course_name`

const courseChatImageJoinsTail = `
	LEFT JOIN rooms pr ON pr.id = r.parent_room_id
	JOIN media md ON md.id = %s.media_id AND md.content_type LIKE 'image/%'
	LEFT JOIN user_accounts u ON u.user_id = md.uploader_user_id`

var courseChatImageSelects = map[model.CourseChatImageSource]string{
	model.CourseChatImageSourceMessage: `
		SELECT 'message' AS source, mm.message_id AS target_id, (m.deleted_at IS NOT NULL) AS target_deleted, ` + courseChatImageColumns + `
		FROM message_media mm
		JOIN messages m ON m.id = mm.message_id
		JOIN rooms r ON r.id = m.room_id AND r.type = 'course'
		` + strings.Replace(courseChatImageJoinsTail, "%s", "mm", 1),
	model.CourseChatImageSourceQuestion: `
		SELECT 'question' AS source, qm.question_id AS target_id, FALSE AS target_deleted, ` + courseChatImageColumns + `
		FROM question_media qm
		JOIN questions q ON q.id = qm.question_id
		JOIN rooms r ON r.id = q.room_id AND r.type = 'course'
		` + strings.Replace(courseChatImageJoinsTail, "%s", "qm", 1),
	model.CourseChatImageSourceAnswer: `
		SELECT 'answer' AS source, am.answer_id AS target_id, FALSE AS target_deleted, ` + courseChatImageColumns + `
		FROM answer_media am
		JOIN answers a ON a.id = am.answer_id
		JOIN questions q ON q.id = a.question_id
		JOIN rooms r ON r.id = q.room_id AND r.type = 'course'
		` + strings.Replace(courseChatImageJoinsTail, "%s", "am", 1),
}

// unionFor は filter に合う添付先だけを UNION ALL で束ねた副問い合わせと、その引数を返す。
func unionFor(filter repository.CourseChatImageFilter) (string, []any) {
	order := []model.CourseChatImageSource{
		model.CourseChatImageSourceMessage, model.CourseChatImageSourceQuestion, model.CourseChatImageSourceAnswer,
	}
	var parts []string
	var args []any
	for _, src := range order {
		if filter.Source != nil && *filter.Source != src {
			continue
		}
		sel := courseChatImageSelects[src]
		if filter.RoomID != nil {
			sel += ` WHERE COALESCE(pr.id, r.id) = ?`
			args = append(args, *filter.RoomID)
		}
		parts = append(parts, sel)
	}
	return strings.Join(parts, " UNION ALL "), args
}

func scanCourseChatImage(sc interface{ Scan(...any) error }) (*model.CourseChatImage, error) {
	var (
		img        model.CourseChatImage
		source     string
		createdAt  int64
		width, hgt sql.NullInt64
		deleted    bool
	)
	if err := sc.Scan(&source, &img.TargetID, &deleted,
		&img.Media.ID, &img.Media.StorageKey, &img.Media.ContentType, &width, &hgt, &createdAt,
		&img.Uploader.ID, &img.Uploader.Name, &img.Uploader.AccountID,
		&img.RoomID, &img.CourseName); err != nil {
		return nil, err
	}
	img.Source = model.CourseChatImageSource(source)
	img.TargetDeleted = deleted
	img.Media.UploaderUserID = img.Uploader.ID
	img.Media.CreatedAt = time.Unix(createdAt, 0)
	if width.Valid {
		w := int(width.Int64)
		img.Media.Width = &w
	}
	if hgt.Valid {
		h := int(hgt.Int64)
		img.Media.Height = &h
	}
	return &img, nil
}

func (r *MySQLCourseChatImageRepository) ListCourseChatImages(ctx context.Context, filter repository.CourseChatImageFilter, limit, offset int) ([]*model.CourseChatImage, int, error) {
	union, args := unionFor(filter)
	if union == "" {
		return nil, 0, nil
	}
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+union+`) t`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.QueryContext(ctx,
		`SELECT * FROM (`+union+`) t ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		append(append([]any{}, args...), limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.CourseChatImage
	for rows.Next() {
		img, err := scanCourseChatImage(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, img)
	}
	return out, total, rows.Err()
}

func (r *MySQLCourseChatImageRepository) GetCourseChatImage(ctx context.Context, mediaID int64) (*model.CourseChatImage, error) {
	union, args := unionFor(repository.CourseChatImageFilter{})
	img, err := scanCourseChatImage(r.DB.QueryRowContext(ctx,
		`SELECT * FROM (`+union+`) t WHERE id = ? LIMIT 1`, append(append([]any{}, args...), mediaID)...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return img, err
}

func (r *MySQLCourseChatImageRepository) DeleteCourseChatImage(ctx context.Context, mediaID int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM media WHERE id = ?`, mediaID)
	return err
}
