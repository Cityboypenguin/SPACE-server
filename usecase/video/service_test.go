package video

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// --- 偽物 --------------------------------------------------------------------

type fakeSettings struct{ values map[string]string }

func defaultSettings() *fakeSettings {
	return &fakeSettings{values: map[string]string{
		SettingRetentionDays:    "90",
		SettingPurgeGraceDays:   "3",
		SettingPurgeBatchSize:   "200",
		SettingDailyUploadLimit: "0",
		SettingDailyBytesLimit:  "0",
		SettingTotalBytesLimit:  "0",
	}}
}

func (f *fakeSettings) Get(_ context.Context, key string) (string, error) { return f.values[key], nil }
func (f *fakeSettings) GetBool(_ context.Context, key string) (bool, error) {
	return f.values[key] == "true", nil
}
func (f *fakeSettings) Update(_ context.Context, key, value string, _ int64) error {
	f.values[key] = value
	return nil
}

type fakeVideos struct {
	repository.VideoRepository // 使わない口は呼ばれたら nil で落ちる

	byID       map[int64]*model.Video
	parents    map[int64]*model.VideoParent
	toPurge    []*model.Video
	purged     []int64
	failures   map[int64]string
	stats      []repository.VideoDailyStats
	expiring   func(cutoff time.Time) repository.VideoUsage
	userUsage  repository.VideoUsage
	stored     repository.VideoUsage
	lastPurgeQ repository.VideoPurgeQuery

	retentions        map[int64]*repository.ReportedVideoRetention
	retentionToPurge  []repository.ReportedVideoRetention
	retentionFailures map[int64]string
	messageVideos     map[int64]*model.Video
}

func (f *fakeVideos) ListRetentionsToPurge(context.Context, time.Time, int) ([]repository.ReportedVideoRetention, error) {
	return f.retentionToPurge, nil
}
func (f *fakeVideos) MarkRetentionPurged(_ context.Context, id int64, at time.Time) error {
	if rv := f.retentions[id]; rv != nil {
		rv.PurgedAt = &at
	}
	return nil
}
func (f *fakeVideos) RecordRetentionPurgeFailure(_ context.Context, id int64, msg string, _ time.Time) error {
	f.retentionFailures[id] = msg
	return nil
}
func (f *fakeVideos) GetRetention(_ context.Context, id int64) (*repository.ReportedVideoRetention, error) {
	return f.retentions[id], nil
}
func (f *fakeVideos) ListVideosByMessageIDs(_ context.Context, ids []int64) (map[int64]*model.Video, error) {
	out := map[int64]*model.Video{}
	for _, id := range ids {
		if v := f.messageVideos[id]; v != nil {
			out[id] = v
		}
	}
	return out, nil
}

func newFakeVideos() *fakeVideos {
	return &fakeVideos{byID: map[int64]*model.Video{}, parents: map[int64]*model.VideoParent{}, failures: map[int64]string{},
		retentions: map[int64]*repository.ReportedVideoRetention{}, retentionFailures: map[int64]string{}, messageVideos: map[int64]*model.Video{}}
}

func (f *fakeVideos) GetVideo(_ context.Context, id int64) (*model.Video, *model.VideoParent, error) {
	return f.byID[id], f.parents[id], nil
}
func (f *fakeVideos) ListVideosToPurge(_ context.Context, q repository.VideoPurgeQuery) ([]*model.Video, error) {
	f.lastPurgeQ = q
	return f.toPurge, nil
}
func (f *fakeVideos) MarkVideoPurged(_ context.Context, id int64, _ time.Time) error {
	f.purged = append(f.purged, id)
	return nil
}
func (f *fakeVideos) RecordVideoPurgeFailure(_ context.Context, id int64, msg string, _ time.Time) error {
	f.failures[id] = msg
	return nil
}
func (f *fakeVideos) AddVideoDailyStats(_ context.Context, d repository.VideoDailyStats) error {
	f.stats = append(f.stats, d)
	return nil
}
func (f *fakeVideos) VideosExpiringBefore(_ context.Context, cutoff time.Time) (repository.VideoUsage, error) {
	return f.expiring(cutoff), nil
}
func (f *fakeVideos) UserVideoUsageSince(context.Context, int64, time.Time) (repository.VideoUsage, error) {
	return f.userUsage, nil
}
func (f *fakeVideos) StoredVideoUsage(context.Context) (repository.VideoUsage, error) {
	return f.stored, nil
}
func (f *fakeVideos) MarkVideoRemoved(_ context.Context, id int64, reason model.VideoRemovedReason, _ int64, at time.Time) error {
	f.byID[id].RemovedAt = &at
	f.byID[id].RemovedReason = &reason
	return nil
}

type fakeStore struct {
	signed    map[string]time.Time
	deleted   []string
	failOnKey string
}

func (f *fakeStore) SignedReadURL(_ context.Context, key string, exp time.Time) (string, error) {
	if f.signed == nil {
		f.signed = map[string]time.Time{}
	}
	f.signed[key] = exp
	return "https://blob.example/" + key, nil
}
func (f *fakeStore) DeleteObject(_ context.Context, key string) error {
	if key == f.failOnKey {
		return errors.New("storage unavailable")
	}
	f.deleted = append(f.deleted, key)
	return nil
}

var testNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

type harness struct {
	svc      *Service
	videos   *fakeVideos
	settings *fakeSettings
	store    *fakeStore
	canView  bool
}

func newHarness() *harness {
	h := &harness{videos: newFakeVideos(), settings: defaultSettings(), store: &fakeStore{}, canView: true}
	h.svc = NewService(Deps{
		Videos:   h.videos,
		Settings: h.settings,
		Store:    h.store,
		CanViewPost: func(context.Context, int64) (bool, error) {
			return h.canView, nil
		},
		CanViewMessage: func(context.Context, int64) (bool, error) {
			return h.canView, nil
		},
		BackstopDays: 120,
		Now:          func() time.Time { return testNow },
	})
	return h
}

func (h *harness) addVideo(id int64, createdAt time.Time) *model.Video {
	v := &model.Video{MediaID: id, UploaderUserID: 1, StorageKey: "videos/1/v.mp4", ThumbnailKey: "video-thumbnails/1/v.jpg", SizeBytes: 1000, CreatedAt: createdAt}
	h.videos.byID[id] = v
	h.videos.parents[id] = &model.VideoParent{Kind: model.VideoParentPost, ID: 10}
	return v
}

func userCtx() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "user"})
}

func adminCtx() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "administrator"})
}

// --- 設定 --------------------------------------------------------------------

// 設定の行が欠けていたら、既定値で動かさずに動画を止める（保存期間をコードに持たない）。
func TestLoadPolicy_RefusesMissingOrBrokenSettings(t *testing.T) {
	for _, mutate := range []func(map[string]string){
		func(m map[string]string) { delete(m, SettingRetentionDays) },
		func(m map[string]string) { m[SettingRetentionDays] = "0" },
		func(m map[string]string) { m[SettingRetentionDays] = "ninety" },
		func(m map[string]string) { m[SettingPurgeBatchSize] = "0" },
		func(m map[string]string) { m[SettingTotalBytesLimit] = "-1" },
	} {
		s := defaultSettings()
		mutate(s.values)
		if _, err := LoadPolicy(context.Background(), s); !errors.Is(err, ErrVideoDisabled) {
			t.Errorf("err = %v, want ErrVideoDisabled", err)
		}
	}
	p, err := LoadPolicy(context.Background(), defaultSettings())
	if err != nil || p.RetentionDays != 90 || p.Retention() != 90*day {
		t.Fatalf("policy = %+v, %v", p, err)
	}
}

func TestMaxRetentionDays(t *testing.T) {
	if got := MaxRetentionDays(120, 3); got != 110 {
		t.Errorf("MaxRetentionDays(120, 3) = %d, want 110", got)
	}
	if got := MaxRetentionDays(0, 3); got != maxRetentionDaysWithoutLimit {
		t.Errorf("without a backstop = %d", got)
	}
}

// --- 期限と状態 --------------------------------------------------------------

func TestVideoStatus(t *testing.T) {
	retention := 90 * day
	created := testNow.Add(-90 * day)
	v := &model.Video{CreatedAt: created}
	if s := v.Status(testNow.Add(-time.Second), retention); s != model.VideoAvailable {
		t.Errorf("1 second before expiry: %s", s)
	}
	if s := v.Status(testNow, retention); s != model.VideoExpired {
		t.Errorf("at expiry: %s", s)
	}
	// 保存期間を延ばせば、まだ消していない動画は再び見える（期限は毎回計算する）。
	if s := v.Status(testNow, 120*day); s != model.VideoAvailable {
		t.Errorf("after extending retention: %s", s)
	}
	at := testNow
	v.RemovedAt = &at
	if s := v.Status(testNow.Add(-day), retention); s != model.VideoRemoved {
		t.Errorf("removed: %s", s)
	}
	v.RemovedAt = nil
	v.ParentDeleted = true
	if s := v.Status(testNow.Add(-day), retention); s != model.VideoRemoved {
		t.Errorf("parent deleted: %s", s)
	}
}

// --- 再生 --------------------------------------------------------------------

func TestRequestPlayback_CapsURLExpiryAtRetentionDeadline(t *testing.T) {
	h := newHarness()
	// あと10分で期限が来る動画。
	h.addVideo(1, testNow.Add(-90*day+10*time.Minute))
	pb, err := h.svc.RequestPlayback(userCtx(), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := testNow.Add(10 * time.Minute)
	if !pb.ExpiresAt.Equal(want) || !h.store.signed["videos/1/v.mp4"].Equal(want) {
		t.Errorf("URL expiry = %v (signed %v), want %v", pb.ExpiresAt, h.store.signed["videos/1/v.mp4"], want)
	}
	if len(h.videos.stats) != 1 || h.videos.stats[0].PlaybackRequests != 1 || h.videos.stats[0].PlaybackBytes != 1000 {
		t.Errorf("stats = %+v", h.videos.stats)
	}
}

// 期限切れの動画には、実体が残っていても URL を出さない（期限切れへの直接アクセス）。
func TestRequestPlayback_RefusesExpired(t *testing.T) {
	h := newHarness()
	h.addVideo(1, testNow.Add(-91*day))
	_, err := h.svc.RequestPlayback(userCtx(), 1)
	if apperr.CodeOf(err) != apperr.CodeNotFound || !strings.Contains(err.Error(), "保存期間（90日間）が終了しました") {
		t.Fatalf("err = %v", err)
	}
	if len(h.store.signed) != 0 {
		t.Error("a URL was signed for an expired video")
	}
}

func TestRequestPlayback_RefusesUnauthorizedViewers(t *testing.T) {
	h := newHarness()
	h.addVideo(1, testNow)
	h.canView = false
	if _, err := h.svc.RequestPlayback(userCtx(), 1); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err = %v, want NOT_FOUND", err)
	}
	if _, err := h.svc.RequestPlayback(context.Background(), 1); err == nil {
		t.Fatal("unauthenticated request got a URL")
	}
	if len(h.store.signed) != 0 {
		t.Error("a URL was signed for an unauthorized viewer")
	}
}

func TestRequestPlayback_RefusesRemoved(t *testing.T) {
	h := newHarness()
	v := h.addVideo(1, testNow)
	v.ParentDeleted = true
	if _, err := h.svc.RequestPlayback(userCtx(), 1); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err = %v", err)
	}
}

// 管理者は通報の確認のため、期限切れでも実体がある限り再生できる。消し終えたものは不可。
func TestRequestPlayback_AdminCanSeeExpiredUntilPurged(t *testing.T) {
	h := newHarness()
	v := h.addVideo(1, testNow.Add(-200*day))
	h.canView = false
	if _, err := h.svc.RequestPlayback(adminCtx(), 1); err != nil {
		t.Fatalf("admin: %v", err)
	}
	at := testNow
	v.PurgedAt = &at
	if _, err := h.svc.RequestPlayback(adminCtx(), 1); err == nil {
		t.Fatal("admin got a URL for a purged video")
	}
}

func TestViewOf_HidesThumbnailWhenNotAvailable(t *testing.T) {
	h := newHarness()
	expired := h.addVideo(1, testNow.Add(-91*day))
	view, err := h.svc.ViewOf(userCtx(), expired)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != model.VideoExpired || view.ThumbnailURL != "" || view.RetentionDays != 90 {
		t.Errorf("view = %+v", view)
	}
	live := h.addVideo(2, testNow.Add(-90*day+30*time.Minute))
	view, _ = h.svc.ViewOf(userCtx(), live)
	if view.ThumbnailURL == "" {
		t.Fatal("thumbnail missing for an available video")
	}
	// サムネイルの URL も期限を越えない。
	if exp := h.store.signed[live.ThumbnailKey]; exp.After(view.ExpiresAt) {
		t.Errorf("thumbnail URL expires %v after the video deadline %v", exp, view.ExpiresAt)
	}
}

// --- 上限 --------------------------------------------------------------------

func TestCheckUploadAllowed_EnforcesConfiguredLimits(t *testing.T) {
	h := newHarness()
	if err := h.svc.CheckUploadAllowed(userCtx()); err != nil {
		t.Fatalf("unlimited: %v", err)
	}
	h.settings.values[SettingDailyUploadLimit] = "3"
	h.svc.invalidate()
	h.videos.userUsage = repository.VideoUsage{Count: 3}
	if err := h.svc.CheckUploadAllowed(userCtx()); apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("daily count limit: err = %v", err)
	}
	h.settings.values[SettingDailyUploadLimit] = "0"
	h.settings.values[SettingTotalBytesLimit] = "100"
	h.svc.invalidate()
	h.videos.stored = repository.VideoUsage{Bytes: 101}
	if err := h.svc.CheckUploadAllowed(userCtx()); apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("total bytes limit: err = %v", err)
	}
}

// --- 実体の削除 --------------------------------------------------------------

func TestPurgeDue_DeletesFilesAndRetriesFailures(t *testing.T) {
	h := newHarness()
	ok := h.addVideo(1, testNow.Add(-100*day))
	bad := h.addVideo(2, testNow.Add(-100*day))
	bad.StorageKey = "videos/1/bad.mp4"
	h.store.failOnKey = bad.StorageKey
	h.videos.toPurge = []*model.Video{bad, ok}

	n, err := h.svc.PurgeDue(context.Background(), testNow)
	if n != 1 || !errors.Is(err, errPurgeIncomplete) {
		t.Fatalf("PurgeDue = %d, %v", n, err)
	}
	if len(h.videos.purged) != 1 || h.videos.purged[0] != ok.MediaID {
		t.Errorf("purged = %v", h.videos.purged)
	}
	if _, recorded := h.videos.failures[bad.MediaID]; !recorded {
		t.Error("failure was not recorded for retry")
	}
	// 動画とサムネイルの両方を消す。
	if strings.Join(h.store.deleted, ",") != ok.StorageKey+","+ok.ThumbnailKey {
		t.Errorf("deleted = %v", h.store.deleted)
	}
	// 期限は「保存期間＋猶予」、外したものは「猶予」で数える。上限は設定の本数。
	q := h.videos.lastPurgeQ
	if !q.ExpiredBefore.Equal(testNow.Add(-93*day)) || !q.RemovedBefore.Equal(testNow.Add(-3*day)) || q.Limit != 200 {
		t.Errorf("purge query = %+v", q)
	}
}

func TestPurgeDue_DoesNothingWithoutSettings(t *testing.T) {
	h := newHarness()
	delete(h.settings.values, SettingRetentionDays)
	h.videos.toPurge = []*model.Video{h.addVideo(1, testNow.Add(-1000*day))}
	if _, err := h.svc.PurgeDue(context.Background(), testNow); !errors.Is(err, ErrVideoDisabled) {
		t.Fatalf("err = %v", err)
	}
	if len(h.store.deleted) != 0 {
		t.Error("files were deleted without settings")
	}
}

// --- 保存期間の変更 ----------------------------------------------------------

func TestRetentionChange_PreviewAndConfirm(t *testing.T) {
	h := newHarness()
	// 登録が cutoff より前の動画を「期限が来ている」と数える偽物。
	// 今の設定（90日）で既に2本、30日にすると5本、30日で1週間後なら8本。
	h.videos.expiring = func(cutoff time.Time) repository.VideoUsage {
		switch {
		case cutoff.Equal(testNow.Add(-90 * day)):
			return repository.VideoUsage{Count: 2, Bytes: 20}
		case cutoff.Equal(testNow.Add(-30 * day)):
			return repository.VideoUsage{Count: 5, Bytes: 50}
		case cutoff.Equal(testNow.Add(-30*day + 7*day)):
			return repository.VideoUsage{Count: 8, Bytes: 80}
		}
		return repository.VideoUsage{}
	}
	p, err := h.svc.PreviewRetention(adminCtx(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if p.NewlyExpired.Count != 3 || p.NewlyExpired.Bytes != 30 || p.ExpiringWithinWeek.Count != 3 || p.MaxDays != 110 {
		t.Fatalf("preview = %+v", p)
	}
	if h.settings.values[SettingRetentionDays] != "90" {
		t.Fatal("preview changed the setting")
	}

	// 確認したときと件数が違えば変えない。
	if _, err := h.svc.UpdateRetention(adminCtx(), 30, 2); apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("stale confirmation: err = %v", err)
	}
	if h.settings.values[SettingRetentionDays] != "90" {
		t.Fatal("stale confirmation changed the setting")
	}
	if _, err := h.svc.UpdateRetention(adminCtx(), 30, 3); err != nil {
		t.Fatal(err)
	}
	if h.settings.values[SettingRetentionDays] != "30" {
		t.Fatalf("retention = %s", h.settings.values[SettingRetentionDays])
	}
	// 変更はキャッシュを捨てて即座に効く。
	if pol, _ := h.svc.Policy(context.Background()); pol.RetentionDays != 30 {
		t.Errorf("policy after update = %d", pol.RetentionDays)
	}
}

func TestRetentionChange_RejectsBeyondBackstopAndNonAdmins(t *testing.T) {
	h := newHarness()
	h.videos.expiring = func(time.Time) repository.VideoUsage { return repository.VideoUsage{} }
	if _, err := h.svc.PreviewRetention(adminCtx(), 111); apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("beyond backstop: err = %v", err)
	}
	if _, err := h.svc.PreviewRetention(userCtx(), 30); err == nil {
		t.Fatal("non-admin could preview")
	}
	if _, err := h.svc.UpdateLimits(adminCtx(), Limits{PurgeGraceDays: 25, PurgeBatchSize: 10}); apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("grace pushing past backstop: err = %v", err)
	}
}
