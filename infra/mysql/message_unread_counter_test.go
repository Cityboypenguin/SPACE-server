package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
	_ "github.com/go-sql-driver/mysql"
)

// 未読の起点（repository.UnreadOrigin の規則）は SQL の CASE 式として組み立てている
// ので、Go 側のロジックを見ても「本当に取りこぼさないか」は分からない。特に
// 「既読更新と新着が同じ秒に起きる」競合は、秒の解像度で比較する SQL を実際に
// 動かさないと再現できない。そのためここは DB を使う統合テストにしてある。
//
// 常用の go test ./... で MySQL を要求したくないので、DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/ -run Unread
//
// スキーマ名は末尾に付けない（テストが使い捨てスキーマを自分で作る）。開発用の
// space スキーマには一切触らない。
func unreadTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed unread count test")
	}

	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open MySQL: %v", err)
	}
	if err := root.Ping(); err != nil {
		root.Close()
		t.Fatalf("failed to reach MySQL: %v", err)
	}

	schema := fmt.Sprintf("space_unread_test_%d", os.Getpid())
	if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to drop the throwaway schema: %v", err)
	}
	if _, err := root.Exec("CREATE DATABASE " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to create the throwaway schema: %v", err)
	}

	db, err := sql.Open("mysql", dsn+schema)
	if err != nil {
		root.Close()
		t.Fatalf("failed to open the throwaway schema: %v", err)
	}

	// db/migrations の DDL から、未読集計が読む列だけを抜き出して外部キーを落とした形。
	// 見たいのは未読の起点の効き方で、参照整合性はここの関心ではない。
	ddl := []string{
		`CREATE TABLE rooms (
			id BIGINT NOT NULL AUTO_INCREMENT,
			type VARCHAR(50) NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE messages (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			deleted_at BIGINT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE room_users (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			last_read_at BIGINT NULL,
			last_read_message_id BIGINT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_room_user (room_id, user_id)
		)`,
		`CREATE TABLE course_room_reads (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			last_read_message_id BIGINT NULL,
			last_read_at BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_course_room_reads_room_user (room_id, user_id)
		)`,
		`CREATE TABLE courses (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			year INT NOT NULL,
			semester VARCHAR(50) NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE timetables (
			id BIGINT NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			course_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			root.Close()
			t.Fatalf("failed to create a test table: %v", err)
		}
	}

	return db, func() {
		db.Close()
		if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
			t.Errorf("failed to clean up the throwaway schema %s: %v", schema, err)
		}
		root.Close()
	}
}

func insertMessage(t *testing.T, db *sql.DB, roomID, userID, createdAt int64) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO messages (room_id, user_id, created_at) VALUES (?, ?, ?)`,
		roomID, userID, createdAt,
	)
	if err != nil {
		t.Fatalf("failed to insert a message: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to read the inserted message id: %v", err)
	}
	return id
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("failed to run %q: %v", query, err)
	}
}

// 既読更新と新着が同じ秒に起きても未読が漏れないこと。
//
// 秒の解像度で created_at > last_read_at を見ていた頃は、既読を打った後に保存された
// メッセージでも「同じ秒」なら未読にならず、二度と未読へ戻らなかった。既読位置を
// メッセージIDにしたので、採番順（AUTO_INCREMENT）で正しく後ろだと判定される。
// ここで確かめているのは「同じ秒」の取りこぼしが消えたことだけで、採番順とコミット順の
// ずれによる競合は別（未解決。repository/read_position.go のコメント参照）。
func TestUnreadCount_SameSecondReadAndNewMessage(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := &MySQLMessageRepository{DB: db}
	roomRepo := &MySQLRoomUserRepository{DB: db}
	courseReadRepo := &MySQLCourseRoomReadRepository{DB: db}

	const (
		dmRoomID     int64 = 1
		courseRoomID int64 = 2
		me           int64 = 10
		partner      int64 = 11
	)
	const now int64 = 1700000000

	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (?, 'dm'), (?, 'course')`, dmRoomID, courseRoomID)
	mustExec(t, db, `INSERT INTO room_users (room_id, user_id) VALUES (?, ?), (?, ?)`, dmRoomID, me, dmRoomID, partner)
	mustExec(t, db, `INSERT INTO courses (id, room_id, year, semester) VALUES (1, ?, 2026, '前期')`, courseRoomID)
	mustExec(t, db, `INSERT INTO timetables (user_id, course_id, created_at) VALUES (?, 1, ?), (?, 1, ?)`, me, now-100, partner, now-100)

	for _, roomID := range []int64{dmRoomID, courseRoomID} {
		// 同じ秒に「相手の投稿 → 自分が既読 → 相手の投稿」が起きた状況。
		insertMessage(t, db, roomID, partner, now)
		latestID, err := repo.GetLatestMessageID(ctx, roomID)
		if err != nil {
			t.Fatalf("failed to resolve the latest message id: %v", err)
		}
		if roomID == courseRoomID {
			if err := courseReadRepo.UpsertLastRead(ctx, roomID, me, latestID, now); err != nil {
				t.Fatalf("failed to record the read position: %v", err)
			}
		} else {
			if err := roomRepo.UpdateLastRead(ctx, roomID, me, latestID, now); err != nil {
				t.Fatalf("failed to record the read position: %v", err)
			}
		}
		insertMessage(t, db, roomID, partner, now)
	}

	// 1ルームぶんの経路（部屋を開いたときの未読表示）。
	dmPosition, err := roomRepo.GetLastRead(ctx, dmRoomID, me)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count, err := repo.CountUnreadMessages(ctx, dmRoomID, me, repository.NewUnreadOrigin(dmPosition, nil)); err != nil || count != 1 {
		t.Fatalf("dm unread = %d (err=%v), want 1 (同じ秒に届いた新着を取りこぼさない)", count, err)
	}
	coursePosition, err := courseReadRepo.GetLastRead(ctx, courseRoomID, me)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count, err := repo.CountUnreadMessages(ctx, courseRoomID, me, repository.NewUnreadOrigin(coursePosition, nil)); err != nil || count != 1 {
		t.Fatalf("course unread = %d (err=%v), want 1", count, err)
	}

	// 一覧のバッジ（JOIN してまとめて数える経路）でも同じこと。
	byRoom, err := repo.CountUnreadMessagesByRoomIDs(ctx, me, []int64{dmRoomID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if byRoom[dmRoomID] != 1 {
		t.Fatalf("dm list badge = %d, want 1", byRoom[dmRoomID])
	}
	if count, err := repo.CountUnreadMessagesByRoomType(ctx, me, "dm"); err != nil || count != 1 {
		t.Fatalf("dm type badge = %d (err=%v), want 1", count, err)
	}
	courseRooms, err := repo.CountUnreadByCourseRooms(ctx, me, 2026, "前期")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(courseRooms) != 1 || courseRooms[0].UnreadCount != 1 {
		t.Fatalf("course list badge = %+v, want 1 unread in one room", courseRooms)
	}

}

// last_read_message_id が NULL のとき（列を足す前からある既読行）は last_read_at 起点。
func TestUnreadCount_FallsBackToTimestampWhenMessageIDIsNull(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := &MySQLMessageRepository{DB: db}
	roomRepo := &MySQLRoomUserRepository{DB: db}

	const (
		roomID  int64 = 1
		me      int64 = 10
		partner int64 = 11
	)
	const now int64 = 1700000000

	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (?, 'community')`, roomID)
	// message ID を持たない既読行（migration 068 適用直後の既存行）。
	mustExec(t, db, `INSERT INTO room_users (room_id, user_id, last_read_at) VALUES (?, ?, ?)`, roomID, me, now)
	mustExec(t, db, `INSERT INTO room_users (room_id, user_id) VALUES (?, ?)`, roomID, partner)

	insertMessage(t, db, roomID, partner, now-10) // 既読前
	insertMessage(t, db, roomID, partner, now+10) // 既読後
	insertMessage(t, db, roomID, me, now+20)      // 自分の発言は数えない

	position, err := roomRepo.GetLastRead(ctx, roomID, me)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if position == nil || position.LastReadMessageID != nil || position.LastReadAt == nil {
		t.Fatalf("read position = %+v, want a timestamp-only position", position)
	}
	if count, err := repo.CountUnreadMessages(ctx, roomID, me, repository.NewUnreadOrigin(position, nil)); err != nil || count != 1 {
		t.Fatalf("unread = %d (err=%v), want 1 (last_read_at 起点)", count, err)
	}
	byRoom, err := repo.CountUnreadMessagesByRoomIDs(ctx, me, []int64{roomID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if byRoom[roomID] != 1 {
		t.Fatalf("list badge = %d, want 1 (last_read_at 起点)", byRoom[roomID])
	}

	// 既読を打てばメッセージIDへ移行し、以降は ID 起点になる。
	latestID, err := repo.GetLatestMessageID(ctx, roomID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := roomRepo.UpdateLastRead(ctx, roomID, me, latestID, now+30); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	migrated, err := roomRepo.GetLastRead(ctx, roomID, me)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated.LastReadMessageID == nil || *migrated.LastReadMessageID != *latestID {
		t.Fatalf("read position = %+v, want the latest message id %d", migrated, *latestID)
	}
	if count, err := repo.CountUnreadMessages(ctx, roomID, me, repository.NewUnreadOrigin(migrated, nil)); err != nil || count != 0 {
		t.Fatalf("unread after reading = %d (err=%v), want 0", count, err)
	}
}

// 既読位置がまったく無いとき、通常ルームは全件・授業ルームは時間割の登録時刻起点。
func TestUnreadCount_NeverReadFallbacks(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := &MySQLMessageRepository{DB: db}

	const (
		communityRoomID int64 = 1
		courseRoomID    int64 = 2
		me              int64 = 10
		partner         int64 = 11
	)
	const now int64 = 1700000000

	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (?, 'community'), (?, 'course')`, communityRoomID, courseRoomID)
	mustExec(t, db, `INSERT INTO room_users (room_id, user_id) VALUES (?, ?), (?, ?)`, communityRoomID, me, communityRoomID, partner)
	mustExec(t, db, `INSERT INTO courses (id, room_id, year, semester) VALUES (1, ?, 2026, '通年')`, courseRoomID)
	// 時間割に登録したのは now。登録前の過去ログは未読に数えない。
	mustExec(t, db, `INSERT INTO timetables (user_id, course_id, created_at) VALUES (?, 1, ?)`, me, now)

	for _, roomID := range []int64{communityRoomID, courseRoomID} {
		insertMessage(t, db, roomID, partner, now-10)
		insertMessage(t, db, roomID, partner, now+10)
	}

	// 一度も読んでいないコミュニティは他人のメッセージを全件未読にする。
	if count, err := repo.CountUnreadMessages(ctx, communityRoomID, me, repository.NewUnreadOrigin(nil, nil)); err != nil || count != 2 {
		t.Fatalf("community unread = %d (err=%v), want 2 (全件)", count, err)
	}
	byRoom, err := repo.CountUnreadMessagesByRoomIDs(ctx, me, []int64{communityRoomID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if byRoom[communityRoomID] != 2 {
		t.Fatalf("community list badge = %d, want 2", byRoom[communityRoomID])
	}

	// 授業は時間割に登録した時刻より後だけ。一覧・部屋内・SSE で同じ数になること。
	registeredAt := now
	if count, err := repo.CountUnreadMessages(ctx, courseRoomID, me, repository.NewUnreadOrigin(nil, &registeredAt)); err != nil || count != 1 {
		t.Fatalf("course unread = %d (err=%v), want 1 (時間割の登録時刻起点)", count, err)
	}
	courseRooms, err := repo.CountUnreadByCourseRooms(ctx, me, 2026, "前期")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(courseRooms) != 1 || courseRooms[0].UnreadCount != 1 {
		t.Fatalf("course list badge = %+v, want 1 unread", courseRooms)
	}
}

// 授業ルームの更新通知の宛先は履修者（timetables）であって room_users ではないこと。
//
// 未読数は数えない。ここが返すのは「誰に知らせるか」だけで、いくつ未読かは
// 知らせを受けた本人が自分ぶんだけ数える（CountUnreadByCourseRooms）。
// 投稿者の除外は配信側（graph/chat_events.go）の担当なので、ここには居る。
func TestListRegistrantIDsByCourseRoomID_TargetsRegistrantsNotRoomUsers(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := &MySQLTimetableRepository{DB: db}

	const (
		courseRoomID int64 = 1
		otherRoomID  int64 = 2
		sender       int64 = 10
		registrant   int64 = 11
		roomUserOnly int64 = 12
		otherCourse  int64 = 13
	)
	const now int64 = 1700000000

	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (?, 'course'), (?, 'course')`, courseRoomID, otherRoomID)
	mustExec(t, db, `INSERT INTO courses (id, room_id, year, semester) VALUES (1, ?, 2026, '前期'), (2, ?, 2026, '前期')`,
		courseRoomID, otherRoomID)
	mustExec(t, db, `INSERT INTO timetables (user_id, course_id, created_at) VALUES (?, 1, ?), (?, 1, ?), (?, 2, ?)`,
		sender, now-100, registrant, now-100, otherCourse, now-100)
	// 授業ルームは room_users を使わない設計だが、万一行があっても宛先にはしない。
	mustExec(t, db, `INSERT INTO room_users (room_id, user_id) VALUES (?, ?)`, courseRoomID, roomUserOnly)
	// 未読の有無で宛先が変わらないこと（messages を見ないクエリであることの裏取り）。
	insertMessage(t, db, courseRoomID, sender, now)

	ids, err := repo.ListRegistrantIDsByCourseRoomID(ctx, courseRoomID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got[sender] || !got[registrant] {
		t.Fatalf("ids = %v, want every registrant of the room (投稿者の除外は配信側の担当)", ids)
	}
	if got[roomUserOnly] {
		t.Fatalf("ids = %v, want room_users rows to be ignored for course rooms", ids)
	}
	if got[otherCourse] {
		t.Fatalf("ids = %v, want registrants of other courses to be excluded", ids)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want exactly the two registrants", ids)
	}
}

// 既読位置は巻き戻さない（別端末が先に進めていればそちらを残す）。
func TestUpdateLastRead_NeverMovesBackwards(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	roomRepo := &MySQLRoomUserRepository{DB: db}
	courseReadRepo := &MySQLCourseRoomReadRepository{DB: db}

	const roomID, userID int64 = 1, 10
	const now int64 = 1700000000
	mustExec(t, db, `INSERT INTO room_users (room_id, user_id) VALUES (?, ?)`, roomID, userID)

	ahead := int64(20)
	behind := int64(5)
	if err := roomRepo.UpdateLastRead(ctx, roomID, userID, &ahead, now+10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := roomRepo.UpdateLastRead(ctx, roomID, userID, &behind, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	position, err := roomRepo.GetLastRead(ctx, roomID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *position.LastReadMessageID != ahead || *position.LastReadAt != now+10 {
		t.Fatalf("read position = %+v, want it kept at message %d / %d", position, ahead, now+10)
	}

	// メッセージが1件も無いルームを既読にしても、既にある位置を消さない。
	if err := roomRepo.UpdateLastRead(ctx, roomID, userID, nil, now+20); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	position, err = roomRepo.GetLastRead(ctx, roomID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if position.LastReadMessageID == nil || *position.LastReadMessageID != ahead {
		t.Fatalf("read position = %+v, want message %d kept", position, ahead)
	}

	// 授業ルーム側（UPSERT）も同じ。
	if err := courseReadRepo.UpsertLastRead(ctx, roomID, userID, &ahead, now+10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := courseReadRepo.UpsertLastRead(ctx, roomID, userID, &behind, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	coursePosition, err := courseReadRepo.GetLastRead(ctx, roomID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *coursePosition.LastReadMessageID != ahead || *coursePosition.LastReadAt != now+10 {
		t.Fatalf("course read position = %+v, want it kept at message %d / %d", coursePosition, ahead, now+10)
	}
}

// 既読位置に使う「最新メッセージID」は削除済みも含めた最大値。末尾が削除済みだからと
// 手前の ID を返すと、その後に来た行が既読位置の後ろに残り既読が巻き戻る。
func TestGetLatestMessageID_IncludesSoftDeleted(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := &MySQLMessageRepository{DB: db}
	const roomID, userID int64 = 1, 10

	if id, err := repo.GetLatestMessageID(ctx, roomID); err != nil || id != nil {
		t.Fatalf("latest id = %v (err=%v), want nil for an empty room", id, err)
	}

	insertMessage(t, db, roomID, userID, 1700000000)
	deletedID := insertMessage(t, db, roomID, userID, 1700000001)
	mustExec(t, db, `UPDATE messages SET deleted_at = ? WHERE id = ?`, 1700000002, deletedID)

	id, err := repo.GetLatestMessageID(ctx, roomID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == nil || *id != deletedID {
		t.Fatalf("latest id = %v, want %d even though it is soft-deleted", id, deletedID)
	}
}

// 069 のバックフィルが期待どおり効くこと（db/migrations/069_... の SQL をそのまま流す）。
//
// 068 で列を足しただけの行（last_read_message_id が NULL）は、次に既読を打つまで
// last_read_at（秒）を起点に数え続ける＝既読と同じ秒の新着を取りこぼす旧挙動が残る。
// 069 はその行を「created_at <= last_read_at を満たす最大の message id」で埋める。
//
// ここで確かめたいのは2つ:
//   - 埋まるべき行だけが埋まること（未読の行・既に ID を持つ行・メッセージが1件も
//     無いルームには触らない）
//   - 埋めても未読数が変わらないこと。移行前の判定は created_at > last_read_at で、
//     既読時刻と同じ秒のメッセージは未読ではなかった。<= で埋めるとその行までが
//     既読位置に入るので、移行後も未読にならない。バッジの数字が動かないことが
//     「安全な移行」の定義なので、前後の count を突き合わせる。
func TestBackfillLastReadMessageID_Migration069(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := &MySQLMessageRepository{DB: db}
	roomRepo := &MySQLRoomUserRepository{DB: db}

	const (
		roomID      int64 = 1
		emptyRoomID int64 = 2
		me          int64 = 10
		partner     int64 = 11
		neverRead   int64 = 12
		alreadyID   int64 = 13
	)
	const readAt int64 = 1700000200

	mustExec(t, db, `INSERT INTO rooms (id, type) VALUES (?, 'dm'), (?, 'dm')`, roomID, emptyRoomID)

	// 既読時刻より前・ちょうど同じ秒・後、の3本。同じ秒の行が既読側に倒れるかが肝。
	insertMessage(t, db, roomID, partner, readAt-100)
	sameSecondID := insertMessage(t, db, roomID, partner, readAt)
	insertMessage(t, db, roomID, partner, readAt+100)

	mustExec(t, db, `INSERT INTO room_users (room_id, user_id, last_read_at, last_read_message_id) VALUES
		(?, ?, ?, NULL),
		(?, ?, NULL, NULL),
		(?, ?, ?, 1),
		(?, ?, ?, NULL)`,
		roomID, me, readAt,
		roomID, neverRead,
		roomID, alreadyID, readAt+100,
		emptyRoomID, me, readAt,
	)

	// 移行前の未読数（last_read_at 起点のフォールバック経路）を控えておく。
	before, err := roomRepo.GetLastRead(ctx, roomID, me)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if before.LastReadMessageID != nil {
		t.Fatalf("precondition broken: the row already has a message id (%v)", before.LastReadMessageID)
	}
	beforeCount, err := repo.CountUnreadMessages(ctx, roomID, me, repository.NewUnreadOrigin(before, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	migration, err := os.ReadFile("../../db/migrations/069_backfill_last_read_message_id_on_room_users.up.sql")
	if err != nil {
		t.Fatalf("failed to read the migration: %v", err)
	}
	if _, err := db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatalf("failed to run the backfill migration: %v", err)
	}

	after, err := roomRepo.GetLastRead(ctx, roomID, me)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if after.LastReadMessageID == nil || *after.LastReadMessageID != sameSecondID {
		t.Fatalf("backfilled position = %v, want the same-second message %d", after.LastReadMessageID, sameSecondID)
	}

	afterCount, err := repo.CountUnreadMessages(ctx, roomID, me, repository.NewUnreadOrigin(after, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if afterCount != beforeCount {
		t.Fatalf("unread count changed by the migration: %d -> %d (移行で未読が増減してはいけない)", beforeCount, afterCount)
	}
	if afterCount != 1 {
		t.Fatalf("unread = %d, want 1 (既読時刻より後の1件だけ)", afterCount)
	}

	// 一度も読んでいない行には触らない（起点が無いのが正しい状態）。
	if pos, err := roomRepo.GetLastRead(ctx, roomID, neverRead); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if pos != nil && pos.LastReadMessageID != nil {
		t.Fatalf("never-read row was backfilled to %v, want NULL", pos.LastReadMessageID)
	}

	// 既に ID を持つ行は進めない（既読位置が勝手に動かない）。
	if pos, err := roomRepo.GetLastRead(ctx, roomID, alreadyID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if pos.LastReadMessageID == nil || *pos.LastReadMessageID != 1 {
		t.Fatalf("existing position = %v, want it left at 1", pos.LastReadMessageID)
	}

	// メッセージが1件も無いルームは MAX(id) が NULL。NULL のままで、
	// 次に届いたメッセージは last_read_at 起点で正しく未読になる。
	if pos, err := roomRepo.GetLastRead(ctx, emptyRoomID, me); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if pos.LastReadMessageID != nil {
		t.Fatalf("empty room position = %v, want NULL", pos.LastReadMessageID)
	}
}

// クライアントが申告してきた既読位置の検証（他ルームのIDを弾く）。
//
// 未読判定は m.id > last_read_message_id という単純比較で、そのIDがどのルームの
// ものかは見ていない。だから「他ルームの大きいID」を1回書き込めるだけで、その
// ルームの未読を丸ごと消せてしまう。検証は SQL 1文なので、ここで実際に流して
// room_id の絞り込みが効いていることを確かめる。
func TestMessageExistsInRoom_ScopesToTheRoomAndIncludesSoftDeleted(t *testing.T) {
	db, cleanup := unreadTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := &MySQLMessageRepository{DB: db}
	const roomID, otherRoomID, userID int64 = 1, 2, 10

	mine := insertMessage(t, db, roomID, userID, 1700000000)
	theirs := insertMessage(t, db, otherRoomID, userID, 1700000001)
	deleted := insertMessage(t, db, roomID, userID, 1700000002)
	mustExec(t, db, `UPDATE messages SET deleted_at = ? WHERE id = ?`, 1700000003, deleted)

	for _, tc := range []struct {
		name      string
		messageID int64
		want      bool
	}{
		{"このルームのメッセージ", mine, true},
		{"他ルームのメッセージ", theirs, false},
		{"存在しないID", theirs + 1000, false},
		// 画面に出したあとに消されたメッセージのIDで既読を打つのは正常な流れ。
		// ここで弾くと「既読にできない部屋」ができる。
		{"論理削除済みのメッセージ", deleted, true},
	} {
		got, err := repo.MessageExistsInRoom(ctx, roomID, tc.messageID)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: MessageExistsInRoom(room=%d, message=%d) = %v, want %v", tc.name, roomID, tc.messageID, got, tc.want)
		}
	}
}
