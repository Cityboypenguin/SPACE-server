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
	md.uploader_user_id, COALESCE(u.name, '退会済みの利用者') AS uploader_name, COALESCE(u.account_id, '') AS uploader_account_id,
	COALESCE(pr.id, r.id) AS room_id, COALESCE(pr.name, r.name) AS course_name`

// courseChatImageOutputColumns は副問い合わせから読む列。scanCourseChatImage と対になっている。
const courseChatImageOutputColumns = `source, target_id, target_deleted, position,
	id, storage_key, content_type, width, height, created_at,
	uploader_user_id, uploader_name, uploader_account_id, room_id, course_name`

// courseChatImageJoins は添付の紐付け表（別名 link）から先の結合。
func courseChatImageJoins(link string) string {
	return `
	LEFT JOIN rooms pr ON pr.id = r.parent_room_id
	JOIN media md ON md.id = ` + link + `.media_id AND md.content_type LIKE 'image/%'
	LEFT JOIN user_accounts u ON u.user_id = md.uploader_user_id`
}

var courseChatImageSelects = map[model.CourseChatImageSource]string{
	model.CourseChatImageSourceMessage: `
		SELECT 'message' AS source, mm.message_id AS target_id, (m.deleted_at IS NOT NULL) AS target_deleted, mm.position AS position, ` + courseChatImageColumns + `
		FROM message_media mm
		JOIN messages m ON m.id = mm.message_id
		JOIN rooms r ON r.id = m.room_id AND r.type = 'course'
		` + courseChatImageJoins("mm"),
	model.CourseChatImageSourceQuestion: `
		SELECT 'question' AS source, qm.question_id AS target_id, FALSE AS target_deleted, qm.position AS position, ` + courseChatImageColumns + `
		FROM question_media qm
		JOIN questions q ON q.id = qm.question_id
		JOIN rooms r ON r.id = q.room_id AND r.type = 'course'
		` + courseChatImageJoins("qm"),
	model.CourseChatImageSourceAnswer: `
		SELECT 'answer' AS source, am.answer_id AS target_id, FALSE AS target_deleted, am.position AS position, ` + courseChatImageColumns + `
		FROM answer_media am
		JOIN answers a ON a.id = am.answer_id
		JOIN questions q ON q.id = a.question_id
		JOIN rooms r ON r.id = q.room_id AND r.type = 'course'
		` + courseChatImageJoins("am"),
}

// unionFor は filter に合う添付先だけを UNION ALL で束ねた副問い合わせと、その引数を返す。
// mediaID を渡すと、その1件だけに絞る（UNION の外で絞ると全画像を集めてからになるので、中で絞る）。
func unionFor(filter repository.CourseChatImageFilter, mediaID *int64) (string, []any) {
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
		var conds []string
		if filter.RoomID != nil {
			conds = append(conds, `COALESCE(pr.id, r.id) = ?`)
			args = append(args, *filter.RoomID)
		}
		if mediaID != nil {
			conds = append(conds, `md.id = ?`)
			args = append(args, *mediaID)
		}
		if len(conds) > 0 {
			sel += ` WHERE ` + strings.Join(conds, ` AND `)
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
	if err := sc.Scan(&source, &img.TargetID, &deleted, &img.Position,
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
	union, args := unionFor(filter, nil)
	if union == "" {
		return nil, 0, nil
	}
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+union+`) t`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.QueryContext(ctx,
		`SELECT `+courseChatImageOutputColumns+` FROM (`+union+`) t ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
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
	union, args := unionFor(repository.CourseChatImageFilter{}, &mediaID)
	img, err := scanCourseChatImage(r.DB.QueryRowContext(ctx,
		`SELECT `+courseChatImageOutputColumns+` FROM (`+union+`) t LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return img, err
}

// removedImageLinks は添付先ごとの紐付け表と親の表の名前。
var removedImageLinks = map[model.CourseChatImageSource]struct{ table, column, parent string }{
	model.CourseChatImageSourceMessage:  {"message_media", "message_id", "messages"},
	model.CourseChatImageSourceQuestion: {"question_media", "question_id", "questions"},
	model.CourseChatImageSourceAnswer:   {"answer_media", "answer_id", "answers"},
}

func (r *MySQLCourseChatImageRepository) RemoveCourseChatImage(ctx context.Context, removed *model.RemovedCourseChatImage) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO removed_course_chat_images
			(media_id, source, target_id, position, room_id, course_name, uploader_user_id, removed_key, original_key,
			 content_type, width, height, created_at, removed_at, removed_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		removed.MediaID, string(removed.Source), removed.TargetID, removed.Position, removed.RoomID, removed.CourseName,
		removed.Uploader.ID, removed.RemovedKey, removed.OriginalKey, removed.ContentType, removed.Width, removed.Height,
		removed.CreatedAt.Unix(), removed.RemovedAt.Unix(), removed.RemovedBy); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media WHERE id = ?`, removed.MediaID); err != nil {
		return err
	}
	return tx.Commit()
}

const removedImageColumns = `ri.media_id, ri.source, ri.target_id, ri.position, ri.room_id, ri.course_name,
	ri.uploader_user_id, COALESCE(u.name, '退会済みの利用者'), COALESCE(u.account_id, ''),
	ri.removed_key, ri.original_key, ri.content_type, ri.width, ri.height, ri.created_at, ri.removed_at, ri.removed_by`

const removedImageFrom = ` FROM removed_course_chat_images ri LEFT JOIN user_accounts u ON u.user_id = ri.uploader_user_id`

func scanRemovedImage(sc interface{ Scan(...any) error }) (*model.RemovedCourseChatImage, error) {
	var (
		img                  model.RemovedCourseChatImage
		source               string
		width, height        sql.NullInt64
		createdAt, removedAt int64
	)
	if err := sc.Scan(&img.MediaID, &source, &img.TargetID, &img.Position, &img.RoomID, &img.CourseName,
		&img.Uploader.ID, &img.Uploader.Name, &img.Uploader.AccountID,
		&img.RemovedKey, &img.OriginalKey, &img.ContentType, &width, &height, &createdAt, &removedAt, &img.RemovedBy); err != nil {
		return nil, err
	}
	img.Source = model.CourseChatImageSource(source)
	img.CreatedAt = time.Unix(createdAt, 0)
	img.RemovedAt = time.Unix(removedAt, 0)
	if width.Valid {
		w := int(width.Int64)
		img.Width = &w
	}
	if height.Valid {
		h := int(height.Int64)
		img.Height = &h
	}
	return &img, nil
}

func collectRemovedImages(rows *sql.Rows) ([]*model.RemovedCourseChatImage, error) {
	defer rows.Close()
	items := []*model.RemovedCourseChatImage{}
	for rows.Next() {
		img, err := scanRemovedImage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, img)
	}
	return items, rows.Err()
}

func (r *MySQLCourseChatImageRepository) ListRemovedCourseChatImages(ctx context.Context, filter repository.CourseChatImageFilter, limit, offset int) ([]*model.RemovedCourseChatImage, int, error) {
	var conds []string
	var args []any
	if filter.RoomID != nil {
		conds = append(conds, `ri.room_id = ?`)
		args = append(args, *filter.RoomID)
	}
	if filter.Source != nil {
		conds = append(conds, `ri.source = ?`)
		args = append(args, string(*filter.Source))
	}
	where := ""
	if len(conds) > 0 {
		where = ` WHERE ` + strings.Join(conds, ` AND `)
	}
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM removed_course_chat_images ri`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+removedImageColumns+removedImageFrom+where+
		` ORDER BY ri.removed_at DESC, ri.media_id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	items, err := collectRemovedImages(rows)
	return items, total, err
}

func (r *MySQLCourseChatImageRepository) GetRemovedCourseChatImage(ctx context.Context, mediaID int64) (*model.RemovedCourseChatImage, error) {
	img, err := scanRemovedImage(r.DB.QueryRowContext(ctx, `SELECT `+removedImageColumns+removedImageFrom+` WHERE ri.media_id = ?`, mediaID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return img, err
}

func (r *MySQLCourseChatImageRepository) ListRemovedCourseChatImagesBefore(ctx context.Context, before time.Time, afterMediaID int64, limit int) ([]*model.RemovedCourseChatImage, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+removedImageColumns+removedImageFrom+
		` WHERE ri.removed_at < ? AND ri.media_id > ? ORDER BY ri.media_id LIMIT ?`, before.Unix(), afterMediaID, limit)
	if err != nil {
		return nil, err
	}
	return collectRemovedImages(rows)
}

func (r *MySQLCourseChatImageRepository) CheckRestorable(ctx context.Context, removed *model.RemovedCourseChatImage) error {
	link, ok := removedImageLinks[removed.Source]
	if !ok {
		return repository.ErrRestoreTargetGone
	}
	cond := ""
	if removed.Source == model.CourseChatImageSourceMessage {
		cond = ` AND deleted_at IS NULL`
	}
	var one int
	err := r.DB.QueryRowContext(ctx, `SELECT 1 FROM `+link.parent+` WHERE id = ?`+cond, removed.TargetID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrRestoreTargetGone
	}
	if err != nil {
		return err
	}
	err = r.DB.QueryRowContext(ctx, `SELECT 1 FROM user_accounts WHERE user_id = ?`, removed.Uploader.ID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrRestoreUploaderGone
	}
	return err
}

func (r *MySQLCourseChatImageRepository) RestoreCourseChatImage(ctx context.Context, removed *model.RemovedCourseChatImage) error {
	link, ok := removedImageLinks[removed.Source]
	if !ok {
		return repository.ErrRestoreTargetGone
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO media (id, uploader_user_id, storage_key, content_type, width, height, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		removed.MediaID, removed.Uploader.ID, removed.OriginalKey, removed.ContentType, removed.Width, removed.Height, removed.CreatedAt.Unix()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO `+link.table+` (`+link.column+`, media_id, position) VALUES (?, ?, ?)`,
		removed.TargetID, removed.MediaID, removed.Position); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM removed_course_chat_images WHERE media_id = ?`, removed.MediaID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLCourseChatImageRepository) DeleteRemovedCourseChatImage(ctx context.Context, mediaID int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM removed_course_chat_images WHERE media_id = ?`, mediaID)
	return err
}
