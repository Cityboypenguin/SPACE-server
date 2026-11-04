package mysql

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"time"
)

// roomAndThreadsMessageSources は、授業・コミュニティのルームと、その配下のスレッドに書かれた
// メッセージを引く2つの FROM〜WHERE 句（メッセージの別名は m）。roomExpr はルームIDの列（例: c.room_id）、
// cond は m に足す条件（" AND m.deleted_at IS NULL" のように先頭に AND を付ける）。
//
// スレッドは親とは別のルームなので、m.room_id = c.room_id だけで数えると、スレッドでどれだけ
// やり取りがあっても「あまり発言されていないルーム」に見える。発言数を数えるところはこれを使う。
// 親とスレッドを OR でひとつの条件にまとめると、MySQL がルームの索引で引けず messages を
// 端からなめるので、別々に引いて足す。cond に ? があるときは引数を2回ぶん渡すこと。
func roomAndThreadsMessageSources(roomExpr, cond string) [2]string {
	return [2]string{
		"FROM messages m WHERE m.room_id = " + roomExpr + cond,
		"FROM rooms tr JOIN messages m ON m.room_id = tr.id WHERE tr.parent_room_id = " + roomExpr + cond,
	}
}

// countRoomAndThreadsMessages はルームと配下のスレッドのメッセージ数を返す式。
func countRoomAndThreadsMessages(roomExpr, cond string) string {
	src := roomAndThreadsMessageSources(roomExpr, cond)
	return "((SELECT COUNT(*) " + src[0] + ") + (SELECT COUNT(*) " + src[1] + "))"
}

// existsRoomAndThreadsMessages はルームか配下のスレッドに該当するメッセージがあるかの条件。
func existsRoomAndThreadsMessages(roomExpr, cond string) string {
	src := roomAndThreadsMessageSources(roomExpr, cond)
	return "(EXISTS (SELECT 1 " + src[0] + ") OR EXISTS (SELECT 1 " + src[1] + "))"
}

type MySQLChatThreadRepository struct{ DB *sql.DB }

var _ repository.ChatThreadRepository = (*MySQLChatThreadRepository)(nil)

func (r *MySQLChatThreadRepository) Create(ctx context.Context, roomID, creatorID int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, "INSERT INTO chat_threads (room_id, created_by) VALUES (?, ?)", roomID, creatorID)
	return err
}

var threadSelect = `SELECT r.id,r.name,r.type,r.created_at,r.updated_at,r.parent_room_id,t.created_by,t.closed_at,
 (SELECT COUNT(*) FROM messages m WHERE m.room_id=r.id AND m.deleted_at IS NULL),
 (SELECT MAX(m.created_at) FROM messages m WHERE m.room_id=r.id AND m.deleted_at IS NULL),
 (SELECT COUNT(*) FROM messages m WHERE m.room_id=r.id AND m.deleted_at IS NULL AND m.user_id<>? AND
  ` + unreadAfterSQL("m", "cr", "NULL") + `)
 FROM chat_threads t JOIN rooms r ON r.id=t.room_id
 LEFT JOIN course_room_reads cr ON cr.room_id=r.id AND cr.user_id=? `

func scanThread(s rowScanner) (*model.ChatThread, error) {
	t := &model.ChatThread{Room: &model.Room{}}
	r := t.Room
	var created, updated int64
	var last *int64
	err := s.Scan(&r.ID, &r.Name, &r.Type, &created, &updated, &r.ParentRoomID, &r.ThreadCreatorID, &r.ThreadClosedAt, &t.MessageCount, &last, &t.UnreadCount)
	r.CreatedAt = time.Unix(created, 0)
	r.UpdatedAt = time.Unix(updated, 0)
	if last != nil {
		v := time.Unix(*last, 0)
		t.LastMessageAt = &v
	}
	return t, err
}
func (r *MySQLChatThreadRepository) Get(ctx context.Context, roomID, viewerID int64) (*model.ChatThread, error) {
	t, err := scanThread(extractDB(ctx, r.DB).QueryRowContext(ctx, threadSelect+" WHERE r.id=?", viewerID, viewerID, roomID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}
func (r *MySQLChatThreadRepository) List(ctx context.Context, parentID, viewerID int64, limit, offset int) ([]*model.ChatThread, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, threadSelect+` WHERE r.parent_room_id=?
 ORDER BY COALESCE((SELECT MAX(m.created_at) FROM messages m WHERE m.room_id=r.id AND m.deleted_at IS NULL),r.created_at) DESC,r.id DESC LIMIT ? OFFSET ?`, viewerID, viewerID, parentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.ChatThread, 0)
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *MySQLChatThreadRepository) Update(ctx context.Context, roomID int64, title string, closed bool) error {
	db := extractDB(ctx, r.DB)
	if _, err := db.ExecContext(ctx, "UPDATE rooms SET name=?,updated_at=? WHERE id=?", title, time.Now().Unix(), roomID); err != nil {
		return err
	}
	var closedAt *int64
	if closed {
		v := time.Now().Unix()
		closedAt = &v
	}
	_, err := db.ExecContext(ctx, "UPDATE chat_threads SET closed_at=? WHERE room_id=?", closedAt, roomID)
	return err
}

// AnnouncementID finds the system post to refresh when the thread is renamed or closed.
func (r *MySQLChatThreadRepository) AnnouncementID(ctx context.Context, roomID int64) (int64, error) {
	var id int64
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, "SELECT id FROM messages WHERE thread_room_id=? AND deleted_at IS NULL ORDER BY id LIMIT 1", roomID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// HasUnread checks every child thread without depending on list pagination.
func (r *MySQLChatThreadRepository) HasUnread(ctx context.Context, parentRoomID, viewerID int64) (bool, error) {
	var unread bool
	query := `SELECT EXISTS (
  SELECT 1 FROM chat_threads t JOIN rooms r ON r.id=t.room_id
  JOIN messages m ON m.room_id=r.id
  LEFT JOIN course_room_reads cr ON cr.room_id=r.id AND cr.user_id=?
  WHERE r.parent_room_id=? AND m.user_id<>? AND m.deleted_at IS NULL AND ` + unreadAfterSQL("m", "cr", "NULL") + `
 )`
	err := extractDB(ctx, r.DB).QueryRowContext(ctx, query, viewerID, parentRoomID, viewerID).Scan(&unread)
	return unread, err
}
