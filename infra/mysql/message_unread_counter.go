package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// repository.MessageUnreadCounter の実装。既読位置そのものは room_users /
// course_room_reads が持ち、ここでは件数を数えるだけ。
//
// 「どこから先を未読と数えるか」は全経路で unreadAfterSQL / unreadAfterCondition
// （infra/mysql/unread_origin.go）に通す。条件式をここへ直接書かないこと。

func (r *MySQLMessageRepository) CountUnreadMessages(ctx context.Context, roomID, userID int64, origin repository.UnreadOrigin) (int, error) {
	after, afterArg := unreadAfterCondition("m", origin)
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM messages m
		WHERE m.room_id = ? AND m.user_id != ? AND m.deleted_at IS NULL AND %s
	`, after)
	var count int
	err := r.DB.QueryRowContext(ctx, query, roomID, userID, afterArg).Scan(&count)
	return count, err
}

func (r *MySQLMessageRepository) CountUnreadMessagesByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int)
	if len(roomIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(`
		SELECT m.room_id, COUNT(*) as unread_count
		FROM messages m
		JOIN room_users ru ON ru.room_id = m.room_id AND ru.user_id = ?
		WHERE m.room_id IN (%s)
		  AND m.user_id != ?
		  AND m.deleted_at IS NULL
		  AND %s
		GROUP BY m.room_id
	`, placeholders, unreadAfterSQL("m", "ru", "NULL"))

	args := make([]interface{}, 0, 2+len(roomIDs))
	args = append(args, userID)
	for _, id := range roomIDs {
		args = append(args, id)
	}
	args = append(args, userID)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var roomID int64
		var count int
		if err := rows.Scan(&roomID, &count); err != nil {
			return nil, err
		}
		result[roomID] = count
	}
	return result, rows.Err()
}

func (r *MySQLMessageRepository) CountUnreadMessagesByRoomType(ctx context.Context, userID int64, roomType string) (int, error) {
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM messages m
		JOIN room_users ru ON ru.room_id = m.room_id AND ru.user_id = ?
		JOIN rooms r ON r.id = m.room_id AND r.type = ?
		WHERE m.user_id != ?
		  AND m.deleted_at IS NULL
		  AND %s
	`, unreadAfterSQL("m", "ru", "NULL"))
	var count int
	err := r.DB.QueryRowContext(ctx, query, userID, roomType, userID).Scan(&count)
	return count, err
}

func (r *MySQLMessageRepository) CountUnreadByCourseRooms(ctx context.Context, userID int64, year int, semester string) ([]*repository.CourseRoomUnread, error) {
	// 「自分の授業」は room_users ではなく時間割から辿る。通年の授業はどちらの学期でも対象。
	// 既読位置が無い（まだ開いていない）授業は、時間割に登録した時点より後を未読として数える。
	// この起点は、部屋を開いたときの未読表示（GetCourseRoomReadStatusUseCase）と
	// 揃えてある（どちらの画面から開いても数が食い違わないように）。
	//
	// このクエリは myCourseRoomUnreadCounts の実体で、「必要になった利用者が自分ぶん
	// だけ取得する」経路そのもの。現在クライアントからは呼ばれていないが、時間割画面に
	// 未読バッジを付けるときの取得口はここになるので残してある（送信のたびに履修者
	// 全員ぶんを数えて配る経路は、誰も読んでいない数字のために重い集計を回すだけ
	// だったので削除した）。
	rows, err := r.DB.QueryContext(ctx, fmt.Sprintf(`
		SELECT c.room_id, COUNT(m.id) AS unread_count
		FROM timetables t
		JOIN courses c ON c.id = t.course_id
		LEFT JOIN course_room_reads cr ON cr.room_id = c.room_id AND cr.user_id = t.user_id
		LEFT JOIN messages m ON m.room_id = c.room_id
		  AND m.user_id <> t.user_id
		  AND m.deleted_at IS NULL
		  AND %s
		WHERE t.user_id = ? AND c.year = ? AND (c.semester = ? OR c.semester = ?)
		GROUP BY c.room_id
		ORDER BY c.room_id
	`, unreadAfterSQL("m", "cr", "t.created_at")), userID, year, semester, model.SemesterFull)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*repository.CourseRoomUnread
	for rows.Next() {
		var unread repository.CourseRoomUnread
		if err := rows.Scan(&unread.RoomID, &unread.UnreadCount); err != nil {
			return nil, err
		}
		result = append(result, &unread)
	}
	return result, rows.Err()
}
