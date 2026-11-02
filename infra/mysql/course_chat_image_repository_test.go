package mysql

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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
		mustExec(t, db, `CREATE TABLE `+tbl.name+` (id BIGINT NOT NULL AUTO_INCREMENT, `+tbl.col+` BIGINT NOT NULL, media_id BIGINT NOT NULL, position INT NOT NULL DEFAULT 0, PRIMARY KEY (id), FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE)`)
	}
	mustExec(t, db, `INSERT INTO user_accounts (user_id, account_id, name) VALUES (10, 'alice', 'Alice')`)
	mustExec(t, db, `INSERT INTO rooms (id, type, name) VALUES (1, 'course', '授業A'), (2, 'course', '授業B'), (3, 'community', 'コミュニティ')`)
	mustExec(t, db, `INSERT INTO rooms (id, type, name, parent_room_id) VALUES (4, 'course', 'スレッド', 1)`)
	migration, err := os.ReadFile("../../db/migrations/125_create_removed_course_chat_images.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, string(migration))
	return &MySQLCourseChatImageRepository{DB: db}, cleanup
}

func removedRecordOf(img *model.CourseChatImage) *model.RemovedCourseChatImage {
	return &model.RemovedCourseChatImage{
		MediaID: img.Media.ID, Source: img.Source, TargetID: img.TargetID, Position: img.Position,
		RoomID: img.RoomID, CourseName: img.CourseName, Uploader: img.Uploader,
		RemovedKey: "removed/x.png", OriginalKey: img.Media.StorageKey, ContentType: img.Media.ContentType,
		Width: img.Media.Width, Height: img.Media.Height, CreatedAt: img.Media.CreatedAt,
		RemovedAt: time.Unix(1000, 0), RemovedBy: 99,
	}
}

func removeForTest(t *testing.T, repo *MySQLCourseChatImageRepository, img *model.CourseChatImage) {
	t.Helper()
	if err := repo.RemoveCourseChatImage(context.Background(), removedRecordOf(img)); err != nil {
		t.Fatal(err)
	}
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
	removeForTest(t, repo, inThread[0])

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
	removeForTest(t, repo, img)
	if _, total, _ := repo.ListCourseChatImages(ctx, repository.CourseChatImageFilter{}, 10, 0); total != 3 {
		t.Errorf("total after delete = %d, want 3", total)
	}
}

func TestCourseChatImageRepository_RemoveAndRestore(t *testing.T) {
	repo, cleanup := courseChatImageTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	db := repo.DB

	mustExec(t, db, `INSERT INTO media (id, uploader_user_id, storage_key, content_type, created_at, width, height) VALUES
		(1, 10, 'k1', 'image/png', 100, 40, 30), (2, 10, 'k2', 'image/png', 200, NULL, NULL)`)
	m := insertMessage(t, db, 1, 10, 1)
	mustExec(t, db, `INSERT INTO message_media (message_id, media_id, position) VALUES (?, 1, 3)`, m)
	mustExec(t, db, `INSERT INTO questions (id, room_id) VALUES (1, 2)`)
	mustExec(t, db, `INSERT INTO question_media (question_id, media_id) VALUES (1, 2)`)

	img, err := repo.GetCourseChatImage(ctx, 1)
	if err != nil || img == nil || img.Position != 3 {
		t.Fatalf("get: %+v %v", img, err)
	}
	removeForTest(t, repo, img)
	if got, _ := repo.GetCourseChatImage(ctx, 1); got != nil {
		t.Fatal("a removed image must no longer be listed for use")
	}
	question, _ := repo.GetCourseChatImage(ctx, 2)
	record := removedRecordOf(question)
	record.RemovedAt = time.Unix(5000, 0)
	if err := repo.RemoveCourseChatImage(ctx, record); err != nil {
		t.Fatal(err)
	}

	all, total, err := repo.ListRemovedCourseChatImages(ctx, repository.CourseChatImageFilter{}, 10, 0)
	if err != nil || total != 2 || all[0].MediaID != 2 || all[1].MediaID != 1 {
		t.Fatalf("list = %+v total=%d err=%v, want newest removal first", all, total, err)
	}
	if all[1].Uploader.Name != "Alice" || *all[1].Width != 40 || all[1].RemovedBy != 99 {
		t.Errorf("record = %+v", all[1])
	}
	room := int64(2)
	if inRoom, n, _ := repo.ListRemovedCourseChatImages(ctx, repository.CourseChatImageFilter{RoomID: &room}, 10, 0); n != 1 || inRoom[0].MediaID != 2 {
		t.Errorf("room filter = %+v", inRoom)
	}
	expired, err := repo.ListRemovedCourseChatImagesBefore(ctx, time.Unix(2000, 0), 0, 10)
	if err != nil || len(expired) != 1 || expired[0].MediaID != 1 {
		t.Errorf("expired = %+v err=%v", expired, err)
	}
	if rest, _ := repo.ListRemovedCourseChatImagesBefore(ctx, time.Unix(9000, 0), 1, 10); len(rest) != 1 || rest[0].MediaID != 2 {
		t.Errorf("after id = %+v", rest)
	}

	// 退会した投稿者の画像は戻せない。行がなくても一覧からは見える。
	mustExec(t, db, `DELETE FROM user_accounts WHERE user_id = 10`)
	removed, _ := repo.GetRemovedCourseChatImage(ctx, 1)
	if removed.Uploader.Name != "退会済みの利用者" {
		t.Errorf("uploader = %+v", removed.Uploader)
	}
	if err := repo.CheckRestorable(ctx, removed); !errors.Is(err, repository.ErrRestoreUploaderGone) {
		t.Errorf("uploader gone: %v", err)
	}
	mustExec(t, db, `INSERT INTO user_accounts (user_id, account_id, name) VALUES (10, 'alice', 'Alice')`)

	// 削除済みのメッセージには戻せない。
	mustExec(t, db, `UPDATE messages SET deleted_at = 1 WHERE id = ?`, m)
	if err := repo.CheckRestorable(ctx, removed); !errors.Is(err, repository.ErrRestoreTargetGone) {
		t.Errorf("message deleted: %v", err)
	}
	mustExec(t, db, `UPDATE messages SET deleted_at = NULL WHERE id = ?`, m)

	if err := repo.CheckRestorable(ctx, removed); err != nil {
		t.Fatal(err)
	}
	if err := repo.RestoreCourseChatImage(ctx, removed); err != nil {
		t.Fatal(err)
	}
	back, err := repo.GetCourseChatImage(ctx, 1)
	if err != nil || back == nil || back.Media.StorageKey != "k1" || back.Position != 3 || back.TargetID != m || *back.Media.Width != 40 {
		t.Fatalf("restored = %+v err=%v", back, err)
	}
	if gone, _ := repo.GetRemovedCourseChatImage(ctx, 1); gone != nil {
		t.Error("the removed record should be gone")
	}

	if err := repo.DeleteRemovedCourseChatImage(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := repo.ListRemovedCourseChatImages(ctx, repository.CourseChatImageFilter{}, 10, 0); n != 0 {
		t.Errorf("remaining = %d", n)
	}
}
