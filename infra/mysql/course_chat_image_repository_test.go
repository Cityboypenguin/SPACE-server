package mysql

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

func courseChatImageTestRepo(t *testing.T) (*MySQLCourseChatImageRepository, func()) {
	t.Helper()
	db, cleanup := unreadTestDB(t)
	mustExec(t, db, `ALTER TABLE rooms ADD COLUMN name VARCHAR(255) NOT NULL DEFAULT '', ADD COLUMN parent_room_id BIGINT NULL`)
	mustExec(t, db, `CREATE TABLE user_accounts (user_id BIGINT NOT NULL, account_id VARCHAR(255) NOT NULL, name VARCHAR(255) NOT NULL, PRIMARY KEY (user_id))`)
	mustExec(t, db, `CREATE TABLE questions (id BIGINT NOT NULL AUTO_INCREMENT, room_id BIGINT NOT NULL, PRIMARY KEY (id))`)
	mustExec(t, db, `CREATE TABLE answers (id BIGINT NOT NULL AUTO_INCREMENT, question_id BIGINT NOT NULL, PRIMARY KEY (id))`)
	mustExec(t, db, `CREATE TABLE media (id BIGINT NOT NULL AUTO_INCREMENT, uploader_user_id BIGINT NOT NULL, storage_key VARCHAR(255) NOT NULL, content_type VARCHAR(100) NOT NULL, created_at BIGINT NOT NULL, width INT NULL, height INT NULL, PRIMARY KEY (id))`)
	for _, tbl := range []struct{ name, col string }{{"message_media", "message_id"}, {"question_media", "question_id"}, {"answer_media", "answer_id"}} {
		mustExec(t, db, `CREATE TABLE `+tbl.name+` (id BIGINT NOT NULL AUTO_INCREMENT, `+tbl.col+` BIGINT NOT NULL, media_id BIGINT NOT NULL, PRIMARY KEY (id), FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE)`)
	}
	mustExec(t, db, `INSERT INTO user_accounts (user_id, account_id, name) VALUES (10, 'alice', 'Alice')`)
	mustExec(t, db, `INSERT INTO rooms (id, type, name) VALUES (1, 'course', '授業A'), (2, 'course', '授業B'), (3, 'community', 'コミュニティ')`)
	mustExec(t, db, `INSERT INTO rooms (id, type, name, parent_room_id) VALUES (4, 'course', 'スレッド', 1)`)
	return &MySQLCourseChatImageRepository{DB: db}, cleanup
}

func TestCourseChatImageRepository_ListFilterAndDelete(t *testing.T) {
	repo, cleanup := courseChatImageTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	db := repo.DB

	// media 1: 授業Aのメッセージ / 2: 授業Aの質問 / 3: 授業Bの回答 / 4: 削除済みメッセージ(授業A)
	// 5: コミュニティのメッセージ(対象外) / 6: 授業Aのメッセージだが画像でない(対象外)
	mustExec(t, db, `INSERT INTO media (id, uploader_user_id, storage_key, content_type, created_at, width, height) VALUES
		(1, 10, 'k1', 'image/png', 100, 40, 30), (2, 10, 'k2', 'image/jpeg', 200, NULL, NULL), (3, 10, 'k3', 'image/png', 300, NULL, NULL),
		(4, 10, 'k4', 'image/png', 400, NULL, NULL), (5, 10, 'k5', 'image/png', 500, NULL, NULL), (6, 10, 'k6', 'application/pdf', 600, NULL, NULL)`)
	m1 := insertMessage(t, db, 1, 10, 1)
	m4 := insertMessage(t, db, 1, 10, 4)
	mustExec(t, db, `UPDATE messages SET deleted_at = 5 WHERE id = ?`, m4)
	m5 := insertMessage(t, db, 3, 10, 5)
	m6 := insertMessage(t, db, 1, 10, 6)
	mustExec(t, db, `INSERT INTO message_media (message_id, media_id) VALUES (?, 1), (?, 4), (?, 5), (?, 6)`, m1, m4, m5, m6)
	mustExec(t, db, `INSERT INTO questions (id, room_id) VALUES (1, 1), (2, 2)`)
	mustExec(t, db, `INSERT INTO question_media (question_id, media_id) VALUES (1, 2)`)
	mustExec(t, db, `INSERT INTO answers (id, question_id) VALUES (1, 2)`)
	mustExec(t, db, `INSERT INTO answer_media (answer_id, media_id) VALUES (1, 3)`)

	all, total, err := repo.ListCourseChatImages(ctx, repository.CourseChatImageFilter{}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || len(all) != 4 {
		t.Fatalf("total=%d len=%d, want 4 (コミュニティと画像以外は除く)", total, len(all))
	}
	// 新しい順。media 4 → 3 → 2 → 1。
	if all[0].Media.ID != 4 || all[1].Media.ID != 3 || all[2].Media.ID != 2 || all[3].Media.ID != 1 {
		t.Errorf("order = %d,%d,%d,%d", all[0].Media.ID, all[1].Media.ID, all[2].Media.ID, all[3].Media.ID)
	}
	if !all[0].TargetDeleted || all[3].TargetDeleted {
		t.Error("targetDeleted must be true only for the deleted message's image")
	}
	if all[1].Source != model.CourseChatImageSourceAnswer || all[1].CourseName != "授業B" || all[1].RoomID != 2 || all[1].TargetID != 1 {
		t.Errorf("answer image = %+v", all[1])
	}
	if all[3].Media.Width == nil || *all[3].Media.Width != 40 || all[3].Uploader.AccountID != "alice" {
		t.Errorf("message image = %+v", all[3])
	}

	room := int64(1)
	inA, total, err := repo.ListCourseChatImages(ctx, repository.CourseChatImageFilter{RoomID: &room}, 10, 0)
	if err != nil || total != 3 || len(inA) != 3 {
		t.Errorf("room filter: total=%d len=%d err=%v, want 3", total, len(inA), err)
	}
	q := model.CourseChatImageSourceQuestion
	onlyQ, total, _ := repo.ListCourseChatImages(ctx, repository.CourseChatImageFilter{Source: &q}, 10, 0)
	if total != 1 || len(onlyQ) != 1 || onlyQ[0].Media.ID != 2 {
		t.Errorf("source filter: %+v", onlyQ)
	}
	page, total, _ := repo.ListCourseChatImages(ctx, repository.CourseChatImageFilter{}, 2, 2)
	if total != 4 || len(page) != 2 || page[0].Media.ID != 2 {
		t.Errorf("paging: total=%d len=%d", total, len(page))
	}

	// スレッドの画像は親の授業の画像として出る。
	mustExec(t, db, `INSERT INTO media (id, uploader_user_id, storage_key, content_type, created_at) VALUES (7, 10, 'k7', 'image/png', 700)`)
	m7 := insertMessage(t, db, 4, 10, 7)
	mustExec(t, db, `INSERT INTO message_media (message_id, media_id) VALUES (?, 7)`, m7)
	inThread, _, err := repo.ListCourseChatImages(ctx, repository.CourseChatImageFilter{RoomID: &room}, 1, 0)
	if err != nil || len(inThread) != 1 || inThread[0].Media.ID != 7 || inThread[0].RoomID != 1 || inThread[0].CourseName != "授業A" {
		t.Errorf("thread image = %+v (err=%v), want it listed under 授業A", inThread, err)
	}
	if err := repo.DeleteCourseChatImage(ctx, 7); err != nil {
		t.Fatal(err)
	}

	if img, err := repo.GetCourseChatImage(ctx, 5); err != nil || img != nil {
		t.Errorf("a community image must not be found: %v %v", img, err)
	}
	if img, err := repo.GetCourseChatImage(ctx, 6); err != nil || img != nil {
		t.Errorf("a non-image must not be found: %v %v", img, err)
	}
	img, err := repo.GetCourseChatImage(ctx, 2)
	if err != nil || img == nil || img.Source != model.CourseChatImageSourceQuestion {
		t.Fatalf("get: %v %v", img, err)
	}
	if err := repo.DeleteCourseChatImage(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := repo.ListCourseChatImages(ctx, repository.CourseChatImageFilter{}, 10, 0); total != 3 {
		t.Errorf("total after delete = %d, want 3", total)
	}
}
