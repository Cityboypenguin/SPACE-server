package video

import (
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 通報の写しには、外した・期限が過ぎた動画も含めてメッセージの動画のキーと実測値を入れる。
func TestSnapshotVideo(t *testing.T) {
	h := newHarness()
	removedAt := testNow.Add(-time.Hour)
	h.videos.messageVideos[7] = &model.Video{MediaID: 3, StorageKey: "videos/1/a.mp4", ThumbnailKey: "video-thumbnails/1/a.jpg",
		SizeBytes: 500, DurationMs: 12_000, Width: 1280, Height: 720, HasAudio: true, CreatedAt: testNow.Add(-100 * day), RemovedAt: &removedAt}

	got, err := h.svc.SnapshotVideo(userCtx(), 7)
	if err != nil || got == nil {
		t.Fatalf("SnapshotVideo = %v, %v", got, err)
	}
	if got.MediaID != 3 || got.StorageKey != "videos/1/a.mp4" || got.ThumbnailKey != "video-thumbnails/1/a.jpg" || got.DurationMs != 12_000 {
		t.Errorf("snapshot = %+v", got)
	}
	if none, err := h.svc.SnapshotVideo(userCtx(), 8); err != nil || none != nil {
		t.Errorf("message without video = %v, %v", none, err)
	}
}

// 管理者は、退会で media の行が消えた動画でも残している間は再生できる。消えたら NOT_FOUND。
func TestRequestReportedPlayback(t *testing.T) {
	h := newHarness()
	rv := &model.ReportedContentVideo{MediaID: 3, StorageKey: "videos/1/a.mp4", ThumbnailKey: "video-thumbnails/1/a.jpg"}

	// 行が無く、残した記録も無い
	if _, err := h.svc.RequestReportedPlayback(adminCtx(), rv); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("no file: err = %v, want NOT_FOUND", err)
	}

	// 退会で残した
	h.videos.retentions[3] = &repository.ReportedVideoRetention{MediaID: 3, StorageKey: rv.StorageKey, ThumbnailKey: rv.ThumbnailKey}
	pb, err := h.svc.RequestReportedPlayback(adminCtx(), rv)
	if err != nil || pb.URL == "" {
		t.Fatalf("retained: %v, %v", pb, err)
	}
	view, err := h.svc.ViewReportedVideo(adminCtx(), rv)
	if err != nil || !view.Available || view.ThumbnailURL == "" {
		t.Fatalf("view = %+v, %v", view, err)
	}

	// 利用者は使えない
	if _, err := h.svc.RequestReportedPlayback(userCtx(), rv); err == nil {
		t.Fatal("non-admin got a playback URL")
	}
	if _, err := h.svc.ViewReportedVideo(userCtx(), rv); err == nil {
		t.Fatal("non-admin got a reported video view")
	}

	// 対応が終わって消した
	purgedAt := testNow
	h.videos.retentions[3].PurgedAt = &purgedAt
	if _, err := h.svc.RequestReportedPlayback(adminCtx(), rv); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("purged: err = %v, want NOT_FOUND", err)
	}
	if view, _ := h.svc.ViewReportedVideo(adminCtx(), rv); view.Available || view.ThumbnailURL != "" {
		t.Fatalf("purged view = %+v", view)
	}

	// 退会していない動画は video_assets の状態に従う
	v := h.addVideo(4, testNow.Add(-200*day))
	rv4 := &model.ReportedContentVideo{MediaID: 4, StorageKey: v.StorageKey}
	if _, err := h.svc.RequestReportedPlayback(adminCtx(), rv4); err != nil {
		t.Fatalf("live expired but not purged: %v", err)
	}
	v.PurgedAt = &purgedAt
	if _, err := h.svc.RequestReportedPlayback(adminCtx(), rv4); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("live purged: err = %v", err)
	}
}

// 退会のあと残した動画は、通報への対応が終わったもの（リポジトリが返すもの）を日次の処理で消す。
// 消せなかったものは記録してやり直す。
func TestPurgeDue_PurgesRetainedReportedVideos(t *testing.T) {
	h := newHarness()
	h.videos.retentions[5] = &repository.ReportedVideoRetention{MediaID: 5, StorageKey: "videos/2/ok.mp4", ThumbnailKey: "video-thumbnails/2/ok.jpg"}
	h.videos.retentions[6] = &repository.ReportedVideoRetention{MediaID: 6, StorageKey: "videos/2/ng.mp4", ThumbnailKey: "video-thumbnails/2/ng.jpg"}
	h.videos.retentionToPurge = []repository.ReportedVideoRetention{*h.videos.retentions[5], *h.videos.retentions[6]}
	h.store.failOnKey = "videos/2/ng.mp4"

	n, err := h.svc.PurgeDue(userCtx(), testNow)
	if n != 1 || err == nil {
		t.Fatalf("PurgeDue = %d, %v; want 1 and a retry error", n, err)
	}
	if h.videos.retentions[5].PurgedAt == nil {
		t.Error("retained video 5 was not marked purged")
	}
	if h.videos.retentions[6].PurgedAt != nil || h.videos.retentionFailures[6] == "" {
		t.Error("failed retained video 6 should be recorded for retry")
	}
	want := map[string]bool{"videos/2/ok.mp4": true, "video-thumbnails/2/ok.jpg": true}
	for _, k := range h.store.deleted {
		delete(want, k)
	}
	if len(want) != 0 {
		t.Errorf("not deleted: %v (deleted %v)", want, h.store.deleted)
	}
}
