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

func (r *MySQLMessageRepository) CountUnreadMessagesPerMember(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	query := fmt.Sprintf(`
		SELECT ru.user_id, COUNT(m.id) as unread_count
		FROM room_users ru
		LEFT JOIN messages m ON m.room_id = ru.room_id
		  AND m.user_id != ru.user_id
		  AND m.deleted_at IS NULL
		  AND %s
		WHERE ru.room_id = ?
		  AND ru.user_id != ?
		GROUP BY ru.user_id
	`, unreadAfterSQL("m", "ru", "NULL"))
	return r.scanUnreadPerUser(ctx, query, roomID, excludeUserID)
}

// CountUnreadMessagesPerCourseRegistrant は授業ルームの未読数を履修者ごとに返す。
//
// 宛先の母集団が room_users ではなく timetables なのが CountUnreadMessagesPerMember
// との唯一の違い。授業内チャットは room_users を持たない設計なので、あちらの SQL では
// 履修者が1人も引っかからず未読の配信先が空になる。
//
// 未読の起点は CountUnreadByCourseRooms（授業一覧のバッジ）と同じ
// 「既読位置 → 無ければ時間割の登録時刻」で、一覧とリアルタイム更新で数が食い違わない。
//
// 履修者ぶんの行が返るが、返すのは (user_id, count) だけなので履修者が多い授業でも
// 転送量は小さい。宛先1人につき1クエリ投げる（N+1）のを避けるためにこの形にしている。
func (r *MySQLMessageRepository) CountUnreadMessagesPerCourseRegistrant(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	query := fmt.Sprintf(`
		SELECT t.user_id, COUNT(m.id) AS unread_count
		FROM timetables t
		JOIN courses c ON c.id = t.course_id AND c.room_id = ?
		LEFT JOIN course_room_reads cr ON cr.room_id = c.room_id AND cr.user_id = t.user_id
		LEFT JOIN messages m ON m.room_id = c.room_id
		  AND m.user_id <> t.user_id
		  AND m.deleted_at IS NULL
		  AND %s
		WHERE t.user_id <> ?
		GROUP BY t.user_id
	`, unreadAfterSQL("m", "cr", "t.created_at"))
	return r.scanUnreadPerUser(ctx, query, roomID, excludeUserID)
}

// scanUnreadPerUser は「利用者ID → 未読数」を返すクエリの実行部分。
// メンバーごと・履修者ごとで母集団だけが違い、読み出しは同じなのでまとめてある。
func (r *MySQLMessageRepository) scanUnreadPerUser(ctx context.Context, query string, args ...interface{}) (map[int64]int, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var userID int64
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}
		result[userID] = count
	}
	return result, rows.Err()
}

func (r *MySQLMessageRepository) CountUnreadByCourseRooms(ctx context.Context, userID int64, year int, semester string) ([]*repository.CourseRoomUnread, error) {
	// 「自分の授業」は room_users ではなく時間割から辿る。通年の授業はどちらの学期でも対象。
	// 既読位置が無い（まだ開いていない）授業は、時間割に登録した時点より後を未読として数える。
	// この起点は、部屋を開いたときの未読表示（GetCourseRoomReadStatusUseCase）とも
	// 未読SSE（CountUnreadMessagesPerCourseRegistrant）とも揃えてある
	// （画面間・画面とリアルタイム更新の間で数が食い違わないように）。
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
