package mysql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	migrationdb "github.com/Cityboypenguin/SPACE-server/db"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 動画のリポジトリを MySQL で確かめる。DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/ -run Video
//
// 見たいのは SQL の条件（期限・外した・親の削除・通報中・やり直しの時刻）なので、
// 外部キーを落とした最小の表で組む。マイグレーションそのものは TestVideoMigrations が見る。
var videoTestDDL = []string{
	`CREATE TABLE media (
		id BIGINT NOT NULL AUTO_INCREMENT, uploader_user_id BIGINT NOT NULL,
		storage_key VARCHAR(255) NOT NULL, content_type VARCHAR(100) NOT NULL,
		created_at BIGINT NOT NULL, width INT NULL, height INT NULL, PRIMARY KEY (id))`,
	`CREATE TABLE video_assets (
		media_id BIGINT NOT NULL, thumbnail_key VARCHAR(255) NOT NULL, size_bytes BIGINT NOT NULL,
		duration_ms INT NOT NULL, fps_milli INT NOT NULL, has_audio TINYINT(1) NOT NULL,
		removed_at BIGINT NULL, removed_reason VARCHAR(20) NULL, removed_by BIGINT NULL,
		purged_at BIGINT NULL, purge_attempts INT NOT NULL DEFAULT 0,
		purge_next_attempt_at BIGINT NULL, purge_last_error VARCHAR(255) NULL, PRIMARY KEY (media_id))`,
	`CREATE TABLE posts (id BIGINT NOT NULL, deleted_at BIGINT NULL, PRIMARY KEY (id))`,
	`CREATE TABLE messages (id BIGINT NOT NULL, deleted_at BIGINT NULL, PRIMARY KEY (id))`,
	`CREATE TABLE post_videos (post_id BIGINT NOT NULL, media_id BIGINT NOT NULL, PRIMARY KEY (post_id), UNIQUE KEY (media_id))`,
	`CREATE TABLE message_videos (message_id BIGINT NOT NULL, media_id BIGINT NOT NULL, PRIMARY KEY (message_id), UNIQUE KEY (media_id))`,
	`CREATE TABLE user_reports (id VARCHAR(255) NOT NULL, target_type VARCHAR(50) NOT NULL,
		target_id VARCHAR(255) NOT NULL, status VARCHAR(50) NOT NULL, PRIMARY KEY (id))
		DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, // 本番（016）と同じ照合順序
	`CREATE TABLE video_daily_stats (day INT NOT NULL, uploads INT NOT NULL DEFAULT 0,
		upload_bytes BIGINT NOT NULL DEFAULT 0, upload_failures INT NOT NULL DEFAULT 0,
		playback_requests INT NOT NULL DEFAULT 0, playback_bytes BIGINT NOT NULL DEFAULT 0,
		purged INT NOT NULL DEFAULT 0, purge_failures INT NOT NULL DEFAULT 0, PRIMARY KEY (day))`,
	`CREATE TABLE reported_video_retentions (media_id BIGINT NOT NULL, storage_key VARCHAR(255) NOT NULL,
		thumbnail_key VARCHAR(255) NOT NULL, size_bytes BIGINT NOT NULL, message_id BIGINT NOT NULL,
		created_at BIGINT NOT NULL, purged_at BIGINT NULL, purge_attempts INT NOT NULL DEFAULT 0,
		purge_next_attempt_at BIGINT NULL, purge_last_error VARCHAR(255) NULL, PRIMARY KEY (media_id))`,
}

func newTestVideo(t *testing.T, repo repository.VideoRepository, createdAt time.Time, size int64) *model.Video {
	t.Helper()
	v := &model.Video{
		UploaderUserID: 1, StorageKey: "videos/1/x.mp4", ThumbnailKey: "video-thumbnails/1/x.jpg",
		SizeBytes: size, DurationMs: 60_000, Width: 1280, Height: 720, FPSMilli: 30_000, HasAudio: true,
		CreatedAt: createdAt,
	}
	if err := repo.CreateVideo(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVideoRepository_PurgeCandidates(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_video", videoTestDDL)
	defer cleanup()
	repo := NewMySQLVideoRepository(db)
	ctx := context.Background()
	now := time.Unix(2_000_000_000, 0)
	day := 24 * time.Hour

	mustExec(t, db, `INSERT INTO posts (id, deleted_at) VALUES (1, NULL), (2, NULL), (3, ?), (4, NULL), (5, NULL)`, now.Add(-10*day).Unix())
	mustExec(t, db, `INSERT INTO messages (id, deleted_at) VALUES (1, NULL)`)

	expired := newTestVideo(t, repo, now.Add(-100*day), 10)   // 期限＋猶予を過ぎた
	fresh := newTestVideo(t, repo, now.Add(-1*day), 20)       // まだ期限前
	parentGone := newTestVideo(t, repo, now.Add(-20*day), 30) // 投稿が10日前に削除された
	reported := newTestVideo(t, repo, now.Add(-100*day), 40)  // 期限は過ぎたが通報中
	removed := newTestVideo(t, repo, now.Add(-5*day), 50)     // 投稿者が外した
	inMessage := newTestVideo(t, repo, now.Add(-100*day), 60) // メッセージの動画、期限切れ

	for postID, v := range map[int64]*model.Video{1: expired, 2: fresh, 3: parentGone, 4: reported, 5: removed} {
		if err := repo.AttachVideoToPost(ctx, postID, v.MediaID); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.AttachVideoToMessage(ctx, 1, inMessage.MediaID); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO user_reports (id, target_type, target_id, status) VALUES ('r1', 'POST', '4', 'PENDING'), ('r2', 'POST', '1', 'RESOLVED')`)
	if err := repo.MarkVideoRemoved(ctx, removed.MediaID, model.VideoRemovedByAuthor, 1, now.Add(-4*day)); err != nil {
		t.Fatal(err)
	}

	q := repository.VideoPurgeQuery{
		Now:           now,
		ExpiredBefore: now.Add(-93 * day), // 保存期間90日＋猶予3日
		RemovedBefore: now.Add(-3 * day),
		Limit:         100,
	}
	got, err := repo.ListVideosToPurge(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, v := range got {
		ids[v.MediaID] = true
	}
	want := map[int64]bool{expired.MediaID: true, parentGone.MediaID: true, removed.MediaID: true, inMessage.MediaID: true}
	if len(ids) != len(want) {
		t.Fatalf("candidates = %v, want %v", ids, want)
	}
	for id := range want {
		if !ids[id] {
			t.Errorf("media %d should be purged", id)
		}
	}
	if ids[fresh.MediaID] || ids[reported.MediaID] {
		t.Error("fresh or reported video was listed")
	}

	// 通報が解決したら対象になる。
	mustExec(t, db, `UPDATE user_reports SET status = 'RESOLVED' WHERE id = 'r1'`)
	got, _ = repo.ListVideosToPurge(ctx, q)
	if len(got) != 5 {
		t.Errorf("after resolving the report: %d candidates, want 5", len(got))
	}

	// 失敗を記録すると、やり直しの時刻までは対象から外れる。
	if err := repo.RecordVideoPurgeFailure(ctx, expired.MediaID, "boom", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.ListVideosToPurge(ctx, q)
	for _, v := range got {
		if v.MediaID == expired.MediaID {
			t.Error("video in retry backoff was listed")
		}
	}
	if n, _ := repo.PurgeFailingCount(ctx); n != 1 {
		t.Errorf("PurgeFailingCount = %d, want 1", n)
	}
	q.Now = now.Add(2 * time.Hour)
	got, _ = repo.ListVideosToPurge(ctx, q)
	found := false
	for _, v := range got {
		found = found || v.MediaID == expired.MediaID
	}
	if !found {
		t.Error("video was not retried after its backoff")
	}

	// 消し終えたものは二度と挙がらず、保存中の量からも外れる。
	before, _ := repo.StoredVideoUsage(ctx)
	if err := repo.MarkVideoPurged(ctx, expired.MediaID, now); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.StoredVideoUsage(ctx)
	if after.Count != before.Count-1 || after.Bytes != before.Bytes-expired.SizeBytes {
		t.Errorf("usage %+v -> %+v", before, after)
	}
	if n, _ := repo.PurgeFailingCount(ctx); n != 0 {
		t.Errorf("PurgeFailingCount after purge = %d, want 0", n)
	}
}

func TestVideoRepository_ReadsAndStatus(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_video_read", videoTestDDL)
	defer cleanup()
	repo := NewMySQLVideoRepository(db)
	ctx := context.Background()
	now := time.Unix(2_000_000_000, 0)

	mustExec(t, db, `INSERT INTO posts (id, deleted_at) VALUES (1, NULL), (2, ?)`, now.Unix())
	mustExec(t, db, `INSERT INTO messages (id, deleted_at) VALUES (7, NULL)`)
	a := newTestVideo(t, repo, now, 100)
	b := newTestVideo(t, repo, now, 200)
	c := newTestVideo(t, repo, now, 300)
	_ = repo.AttachVideoToPost(ctx, 1, a.MediaID)
	_ = repo.AttachVideoToPost(ctx, 2, b.MediaID)
	_ = repo.AttachVideoToMessage(ctx, 7, c.MediaID)

	byPost, err := repo.ListVideosByPostIDs(ctx, []int64{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(byPost) != 2 || byPost[1].SizeBytes != 100 || byPost[1].ParentDeleted || !byPost[2].ParentDeleted {
		t.Fatalf("byPost = %+v", byPost)
	}
	if byPost[3] != nil {
		t.Error("post without a video returned one")
	}
	byMsg, _ := repo.ListVideosByMessageIDs(ctx, []int64{7})
	if byMsg[7] == nil || byMsg[7].ThumbnailKey != "video-thumbnails/1/x.jpg" || byMsg[7].Width != 1280 {
		t.Fatalf("byMsg = %+v", byMsg)
	}

	v, parent, err := repo.GetVideo(ctx, c.MediaID)
	if err != nil || v == nil || parent == nil || parent.Kind != model.VideoParentMessage || parent.ID != 7 {
		t.Fatalf("GetVideo = %+v, %+v, %v", v, parent, err)
	}
	if v, _, _ := repo.GetVideo(ctx, 9999); v != nil {
		t.Error("missing video returned a row")
	}

	// 外した理由は最初のものが残る。
	_ = repo.MarkVideoRemoved(ctx, a.MediaID, model.VideoRemovedByAuthor, 1, now)
	_ = repo.MarkVideoRemoved(ctx, a.MediaID, model.VideoRemovedByAdmin, 99, now.Add(time.Hour))
	v, _, _ = repo.GetVideo(ctx, a.MediaID)
	if v.RemovedReason == nil || *v.RemovedReason != model.VideoRemovedByAuthor {
		t.Errorf("removed reason = %v", v.RemovedReason)
	}

	// 期限の来ている本数（保存期間の変更の影響確認）は、外したもの・親が消えたものを数えない。
	u, err := repo.VideosExpiringBefore(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if u.Count != 1 || u.Bytes != 300 {
		t.Errorf("VideosExpiringBefore = %+v, want only the message video", u)
	}
	if u, _ := repo.UserVideoUsageSince(ctx, 1, now); u.Count != 3 || u.Bytes != 600 {
		t.Errorf("UserVideoUsageSince = %+v", u)
	}
}

func TestVideoRepository_DailyStats(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_video_stats", videoTestDDL)
	defer cleanup()
	repo := NewMySQLVideoRepository(db)
	ctx := context.Background()
	for range 2 {
		if err := repo.AddVideoDailyStats(ctx, repository.VideoDailyStats{Day: 20261120, Uploads: 1, UploadBytes: 10, PlaybackRequests: 2}); err != nil {
			t.Fatal(err)
		}
	}
	_ = repo.AddVideoDailyStats(ctx, repository.VideoDailyStats{Day: 20261121, Purged: 3})
	got, err := repo.ListVideoDailyStats(ctx, 20261120, 20261121)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Uploads != 2 || got[0].UploadBytes != 20 || got[0].PlaybackRequests != 4 || got[1].Purged != 3 {
		t.Fatalf("stats = %+v", got)
	}
}

// 動画の行は「添付の削除」（投稿の編集の deletedMediaIDs）で消えないこと。
// 消えると video_assets ごと消え、ファイルが日次の削除から漏れて残り続ける。
func TestMediaRepository_DoesNotDeleteVideoRows(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_video_media", videoTestDDL)
	defer cleanup()
	videos := NewMySQLVideoRepository(db)
	media := NewMySQLMediaRepository(db)
	ctx := context.Background()
	v := newTestVideo(t, videos, time.Now(), 1)
	if err := media.DeleteMediaByIDAndUserID(ctx, v.MediaID, 1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media WHERE id = ?`, v.MediaID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("video media row was deleted (count=%d, err=%v)", n, err)
	}
	keys, err := media.ListStorageKeysByUploader(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != v.StorageKey || keys[1] != v.ThumbnailKey {
		t.Errorf("keys = %v, want the video and its thumbnail", keys)
	}
}

// マイグレーション 130〜135 が最新まで通り、戻して入れ直せること。
func TestVideoMigrations(t *testing.T) {
	database, cleanup := throwawaySchemaDB(t, "space_video_migrations", nil)
	defer cleanup()
	if err := migrationdb.RunMigrations(database); err != nil {
		t.Fatal(err)
	}
	var retention string
	if err := database.QueryRow("SELECT value FROM system_settings WHERE `key` = 'video_retention_days'").Scan(&retention); err != nil || retention != "90" {
		t.Fatalf("video_retention_days = %q, %v", retention, err)
	}
	// 本物の表（照合順序・外部キー込み）で、動画のクエリがそのまま通ること。
	// 最小の表で組んだ他のテストでは、列の照合順序の違いなどを見落とすため。
	repo := NewMySQLVideoRepository(database)
	ctx := context.Background()
	now := time.Now()
	if _, err := repo.ListVideosToPurge(ctx, repository.VideoPurgeQuery{Now: now, ExpiredBefore: now, RemovedBefore: now, Limit: 10}); err != nil {
		t.Fatalf("ListVideosToPurge on the migrated schema: %v", err)
	}
	if _, err := repo.ListRetentionsToPurge(ctx, now, 10); err != nil {
		t.Fatalf("ListRetentionsToPurge on the migrated schema: %v", err)
	}
	if _, err := repo.RetainReportedMessageVideos(ctx, 1, now); err != nil {
		t.Fatalf("RetainReportedMessageVideos on the migrated schema: %v", err)
	}
	for _, f := range []func(context.Context) (repository.VideoUsage, error){repo.StoredVideoUsage} {
		if _, err := f(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.PurgeFailingCount(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.VideosExpiringBefore(ctx, now); err != nil {
		t.Fatal(err)
	}
	apply := func(name string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(string(b)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, n := range []string{"135_create_reported_video_retentions", "134_create_video_daily_stats", "133_insert_default_video_settings", "132_create_message_videos", "131_create_post_videos", "130_create_video_assets"} {
		apply(n + ".down.sql")
	}
	for _, n := range []string{"130_create_video_assets", "131_create_post_videos", "132_create_message_videos", "133_insert_default_video_settings", "134_create_video_daily_stats", "135_create_reported_video_retentions"} {
		apply(n + ".up.sql")
	}
}

// 退会のとき、通報中のメッセージの動画だけを残す記録を作り、対応が終わったら削除の対象になる。
func TestVideoRepository_RetainReportedMessageVideos(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_video_retain", videoTestDDL)
	defer cleanup()
	repo := NewMySQLVideoRepository(db)
	ctx := context.Background()
	now := time.Unix(2_000_000_000, 0)

	mustExec(t, db, `INSERT INTO messages (id, deleted_at) VALUES (1, NULL), (2, NULL), (3, NULL)`)
	reported := newTestVideo(t, repo, now.Add(-time.Hour), 10)   // 通報中のメッセージ
	resolved := newTestVideo(t, repo, now.Add(-time.Hour), 20)   // 通報は対応済み
	unreported := newTestVideo(t, repo, now.Add(-time.Hour), 30) // 通報なし
	other := &model.Video{UploaderUserID: 2, StorageKey: "videos/2/y.mp4", ThumbnailKey: "video-thumbnails/2/y.jpg",
		SizeBytes: 5, CreatedAt: now} // 別の人の動画
	if err := repo.CreateVideo(ctx, other); err != nil {
		t.Fatal(err)
	}
	for msgID, v := range map[int64]*model.Video{1: reported, 2: resolved, 3: unreported} {
		if err := repo.AttachVideoToMessage(ctx, msgID, v.MediaID); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(t, db, `UPDATE media SET storage_key = CONCAT('videos/1/', id, '.mp4') WHERE uploader_user_id = 1`)
	mustExec(t, db, `INSERT INTO user_reports (id, target_type, target_id, status) VALUES
		('r1', ?, '1', 'reviewing'), ('r2', ?, '2', 'DISMISSED')`, string(model.TargetMessage), string(model.TargetMessage))

	keys, err := repo.RetainReportedMessageVideos(ctx, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	wantKey := fmt.Sprintf("videos/1/%d.mp4", reported.MediaID)
	if len(keys) != 2 || keys[0] != wantKey || keys[1] != "video-thumbnails/1/x.jpg" {
		t.Fatalf("kept keys = %v, want [%s video-thumbnails/1/x.jpg]", keys, wantKey)
	}
	// 2回呼んでも記録は1つ（退会のやり直し）。
	if again, err := repo.RetainReportedMessageVideos(ctx, 1, now); err != nil || len(again) != 2 {
		t.Fatalf("second call = %v, %v", again, err)
	}

	// 退会で media の行が消えても残る（本番は video_assets も CASCADE で消える。この表には外部キーが無いので手で消す）。
	mustExec(t, db, `DELETE va FROM video_assets va JOIN media m ON m.id = va.media_id WHERE m.uploader_user_id = 1`)
	mustExec(t, db, `DELETE FROM media WHERE uploader_user_id = 1`)
	rv, err := repo.GetRetention(ctx, reported.MediaID)
	if err != nil || rv == nil || rv.StorageKey != wantKey || rv.MessageID != 1 || rv.SizeBytes != 10 {
		t.Fatalf("retention = %+v, %v", rv, err)
	}
	if usage, _ := repo.StoredVideoUsage(ctx); usage.Count != 2 || usage.Bytes != 15 {
		t.Errorf("stored usage = %+v, want 2 videos / 15 bytes (other + retained)", usage)
	}

	// 通報中は消さない。
	if due, err := repo.ListRetentionsToPurge(ctx, now, 10); err != nil || len(due) != 0 {
		t.Fatalf("while reported: %v, %v", due, err)
	}
	// 対応が終わったら消す対象になる。失敗したらやり直しの時刻まで出てこない。
	mustExec(t, db, `UPDATE user_reports SET status = 'RESOLVED' WHERE id = 'r1'`)
	due, err := repo.ListRetentionsToPurge(ctx, now, 10)
	if err != nil || len(due) != 1 || due[0].MediaID != reported.MediaID {
		t.Fatalf("after resolving: %v, %v", due, err)
	}
	if err := repo.RecordRetentionPurgeFailure(ctx, reported.MediaID, "boom", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.PurgeFailingCount(ctx); n != 1 {
		t.Errorf("PurgeFailingCount = %d, want 1", n)
	}
	if due, _ := repo.ListRetentionsToPurge(ctx, now, 10); len(due) != 0 {
		t.Errorf("listed before the retry time: %v", due)
	}
	if due, _ := repo.ListRetentionsToPurge(ctx, now.Add(time.Hour), 10); len(due) != 1 {
		t.Errorf("not listed at the retry time: %v", due)
	}
	if err := repo.MarkRetentionPurged(ctx, reported.MediaID, now); err != nil {
		t.Fatal(err)
	}
	rv, _ = repo.GetRetention(ctx, reported.MediaID)
	if rv == nil || rv.PurgedAt == nil {
		t.Fatalf("not marked purged: %+v", rv)
	}
	if due, _ := repo.ListRetentionsToPurge(ctx, now.Add(2*time.Hour), 10); len(due) != 0 {
		t.Errorf("purged retention listed again: %v", due)
	}
	if missing, err := repo.GetRetention(ctx, 999); err != nil || missing != nil {
		t.Errorf("missing retention = %v, %v", missing, err)
	}
}
