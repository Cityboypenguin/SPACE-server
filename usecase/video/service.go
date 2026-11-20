package video

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

// Store は動画の置き場のうち、ここで使う口。
type Store interface {
	// SignedReadURL は expiresAt まで読める署名付きURLを作る。
	SignedReadURL(ctx context.Context, objectKey string, expiresAt time.Time) (string, error)
	DeleteObject(ctx context.Context, objectKey string) error
}

// RateLimiter は署名付きURLを出す回数の制限（短時間に大量に出させない）。
type RateLimiter interface {
	Allow(ctx context.Context, userID int64) (bool, error)
}

type Deps struct {
	Videos   repository.VideoRepository
	Settings repository.SystemSettingRepository
	Store    Store
	// CanViewPost / CanViewMessage は、呼び出し元がその投稿・メッセージを見てよいか。
	// 動画の再生はこれに従う（投稿のブロック・退会、ルームのメンバーかどうか）。
	CanViewPost    func(ctx context.Context, postID int64) (bool, error)
	CanViewMessage func(ctx context.Context, messageID int64) (bool, error)
	// UploadRate は nil なら制限しない。
	UploadRate RateLimiter
	// BackstopDays は Azure のライフサイクル管理が動画を消す日数（安全網）。
	// 0 なら安全網を置いていない（ローカル環境など）。
	BackstopDays int
	Now          func() time.Time
}

// Service は動画の扱いの入口。cmd/server で1つ作って配る。
type Service struct {
	d Deps

	mu       sync.Mutex
	cached   *Policy
	cachedAt time.Time
}

func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

// policyCacheTTL は設定を読み直す間隔。一覧の動画1本ごとに system_settings を
// 6回引かないため。管理画面からの変更は Invalidate で即座に効く（サーバーは1台）。
const policyCacheTTL = 30 * time.Second

// Policy は現在の設定。
func (s *Service) Policy(ctx context.Context) (Policy, error) {
	s.mu.Lock()
	if s.cached != nil && s.d.Now().Sub(s.cachedAt) < policyCacheTTL {
		p := *s.cached
		s.mu.Unlock()
		return p, nil
	}
	s.mu.Unlock()

	p, err := LoadPolicy(ctx, s.d.Settings)
	if err != nil {
		return Policy{}, err
	}
	s.mu.Lock()
	s.cached, s.cachedAt = &p, s.d.Now()
	s.mu.Unlock()
	return p, nil
}

// BackstopDays は Azure のライフサイクル管理が動画を消す日数（0 は未設定）。
func (s *Service) BackstopDays() int { return s.d.BackstopDays }

func (s *Service) invalidate() {
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
}

// disabledErr は設定の欠けを利用者向けの文言に置き換え、詳細はログに残す。
func disabledErr(err error) error {
	if errors.Is(err, ErrVideoDisabled) {
		logger.Log.Error().Err(err).Str("component", "video").Msg("video settings are missing; videos are disabled")
		return apperr.InvalidInput(ErrVideoDisabled.Error())
	}
	return err
}

// --- 受け入れ ----------------------------------------------------------------

// CheckUploadAllowed は署名付きURLを出してよいかを見る（出す前に分かる上限だけ）。
// 大きさの上限は実物を測るまで分からないので、受け入れ（Accept）でもう一度見る。
func (s *Service) CheckUploadAllowed(ctx context.Context) error {
	callerID, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	p, err := s.Policy(ctx)
	if err != nil {
		return disabledErr(err)
	}
	if err := s.checkQuota(ctx, p, callerID, 0); err != nil {
		return err
	}
	if s.d.UploadRate != nil {
		ok, err := s.d.UploadRate.Allow(ctx, callerID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.InvalidInput("動画のアップロードが短時間に集中しています。しばらく待ってからやり直してください")
		}
	}
	return nil
}

// checkQuota は1人1日の上限と全体の容量上限を見る。addBytes はこれから足す大きさ。
func (s *Service) checkQuota(ctx context.Context, p Policy, userID, addBytes int64) error {
	now := s.d.Now()
	if p.DailyUploadLimit > 0 || p.DailyBytesLimit > 0 {
		used, err := s.d.Videos.UserVideoUsageSince(ctx, userID, startOfDayJST(now))
		if err != nil {
			return err
		}
		if p.DailyUploadLimit > 0 && used.Count+1 > p.DailyUploadLimit {
			return apperr.InvalidInput(fmt.Sprintf("動画は1日%d本まで投稿できます", p.DailyUploadLimit))
		}
		if p.DailyBytesLimit > 0 && used.Bytes+addBytes > p.DailyBytesLimit {
			return apperr.InvalidInput("今日投稿できる動画の容量の上限に達しました")
		}
	}
	if p.TotalBytesLimit > 0 {
		stored, err := s.d.Videos.StoredVideoUsage(ctx)
		if err != nil {
			return err
		}
		if stored.Bytes+addBytes > p.TotalBytesLimit {
			return apperr.InvalidInput("動画の保存容量が上限に達しているため、現在は投稿できません")
		}
	}
	return nil
}

// Accept は動画とサムネイルを受け入れ、保存用の Video を組み立てる。
//
// 受け入れたファイルは uploads が覚えるので、この後の保存が失敗したら呼び出し側の
// DiscardOnError で消える（孤立したファイルを残さない）。ここで上限に当たった
// 場合も、エラーを返せば同じく消える。
func (s *Service) Accept(ctx context.Context, uploads *uploadusecase.Session, in model.VideoInput) (_ *model.Video, err error) {
	callerID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.Policy(ctx)
	if err != nil {
		return nil, disabledErr(err)
	}
	if in.StorageKey == "" || in.ThumbnailKey == "" {
		return nil, apperr.InvalidInput("動画とサムネイルの両方が必要です")
	}
	defer func() {
		if err != nil && apperr.CodeOf(err) == apperr.CodeInvalidInput {
			s.addStats(ctx, repository.VideoDailyStats{UploadFailures: 1})
		}
	}()

	// 本数の上限は、ファイルに触る前に見る（上限に当たっているのに検査で CPU を使わない）。
	if err := s.checkQuota(ctx, p, callerID, 0); err != nil {
		return nil, err
	}
	key, probe, err := uploads.AcceptVideo(ctx, in.StorageKey)
	if err != nil {
		return nil, err
	}
	thumb, err := uploads.Accept(ctx, uploadusecase.VideoThumbnail, in.ThumbnailKey)
	if err != nil {
		return nil, err
	}
	if err := s.checkQuota(ctx, p, callerID, probe.SizeBytes); err != nil {
		return nil, err
	}
	return &model.Video{
		UploaderUserID: callerID,
		StorageKey:     key,
		ThumbnailKey:   thumb,
		SizeBytes:      probe.SizeBytes,
		DurationMs:     probe.DurationMs,
		Width:          probe.Width,
		Height:         probe.Height,
		FPSMilli:       probe.FPSMilli,
		HasAudio:       probe.HasAudio,
		CreatedAt:      s.d.Now(),
	}, nil
}

// SaveForPost は受け入れた動画を投稿に付けて保存する。投稿の保存と同じトランザクションで呼ぶ。
func (s *Service) SaveForPost(ctx context.Context, postID int64, v *model.Video) error {
	if err := s.d.Videos.CreateVideo(ctx, v); err != nil {
		return err
	}
	return s.d.Videos.AttachVideoToPost(ctx, postID, v.MediaID)
}

// SaveForMessage は受け入れた動画をメッセージに付けて保存する。
func (s *Service) SaveForMessage(ctx context.Context, messageID int64, v *model.Video) error {
	if err := s.d.Videos.CreateVideo(ctx, v); err != nil {
		return err
	}
	return s.d.Videos.AttachVideoToMessage(ctx, messageID, v.MediaID)
}

// RecordSaved は保存が確定した動画を集計に足す。トランザクションの外（確定後）で呼ぶ。
func (s *Service) RecordSaved(ctx context.Context, v *model.Video) {
	s.addStats(ctx, repository.VideoDailyStats{Uploads: 1, UploadBytes: v.SizeBytes})
}

// RemoveFromPost は投稿者が編集で外した動画を見えなくする（実体は日次の処理で消す）。
// 動画が付いていなければ何もしない。投稿者本人かどうかは呼び出し側（投稿の編集）が
// 確かめ済みだが、動画の持ち主とも一致することをここでも見る。
func (s *Service) RemoveFromPost(ctx context.Context, postID, userID int64) error {
	videos, err := s.d.Videos.ListVideosByPostIDs(ctx, []int64{postID})
	if err != nil {
		return err
	}
	v := videos[postID]
	if v == nil {
		return nil
	}
	if v.UploaderUserID != userID {
		return apperr.Forbidden("この動画は外せません")
	}
	return s.d.Videos.MarkVideoRemoved(ctx, v.MediaID, model.VideoRemovedByAuthor, userID, s.d.Now())
}

// addStats は集計を足す。失敗しても本体の処理は止めない（集計は目安なので）。
func (s *Service) addStats(ctx context.Context, delta repository.VideoDailyStats) {
	delta.Day = dayKey(s.d.Now())
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.d.Videos.AddVideoDailyStats(ctx, delta); err != nil {
		logger.Log.Error().Err(err).Str("component", "video").Msg("failed to record video stats")
	}
}

// ListByPostIDs / ListByMessageIDs は DataLoader 用の一括取得。
func (s *Service) ListByPostIDs(ctx context.Context, ids []int64) (map[int64]*model.Video, error) {
	return s.d.Videos.ListVideosByPostIDs(ctx, ids)
}

func (s *Service) ListByMessageIDs(ctx context.Context, ids []int64) (map[int64]*model.Video, error) {
	return s.d.Videos.ListVideosByMessageIDs(ctx, ids)
}

// --- 表示と再生 --------------------------------------------------------------

// View は利用者に見せる動画の状態。
type View struct {
	Video         *model.Video
	Status        model.VideoStatus
	ExpiresAt     time.Time
	RetentionDays int
	// ThumbnailURL は再生できる状態のときだけ入る。
	ThumbnailURL string
}

// thumbnailURLTTL / thumbnailURLWindow はサムネイルの署名付きURLの寿命と丸めの幅。
// 丸めておくと、同じ窓の中では同じURLになり、ブラウザーのキャッシュが効く。
const (
	thumbnailURLTTL    = time.Hour
	thumbnailURLWindow = time.Hour
	// playbackURLTTL は再生用URLの寿命。再生を始めた後のシーク（Range の要求）にも
	// 同じURLを使うので、動画の長さ（60秒）より十分長くとる。
	playbackURLTTL = time.Hour
)

// ViewOf は v の今の状態を返す。管理者には、外したもの・期限が過ぎたものでも実体が
// 残っている限りサムネイルを返す（通報の確認のため）。
func (s *Service) ViewOf(ctx context.Context, v *model.Video) (View, error) {
	p, err := s.Policy(ctx)
	if err != nil {
		return View{}, disabledErr(err)
	}
	now := s.d.Now()
	view := View{
		Video:         v,
		Status:        v.Status(now, p.Retention()),
		ExpiresAt:     v.ExpiresAt(p.Retention()),
		RetentionDays: p.RetentionDays,
	}
	admin := isAdmin(ctx)
	if view.Status != model.VideoAvailable && !(admin && v.PurgedAt == nil) {
		return view, nil
	}
	expiry := now.Add(thumbnailURLTTL).Truncate(thumbnailURLWindow).Add(thumbnailURLWindow)
	if !admin && view.ExpiresAt.Before(expiry) {
		expiry = view.ExpiresAt
	}
	url, err := s.d.Store.SignedReadURL(ctx, v.ThumbnailKey, expiry)
	if err != nil {
		// サムネイルが出ないだけで、他の項目は返す。
		logger.Log.Error().Err(err).Str("component", "video").Int64("media_id", v.MediaID).Msg("failed to sign a thumbnail URL")
		return view, nil
	}
	view.ThumbnailURL = url
	return view, nil
}

func isAdmin(ctx context.Context) bool {
	_, err := authz.RequireAdmin(ctx)
	return err == nil
}

// Playback は再生用のURLとその期限。
type Playback struct {
	URL       string
	ExpiresAt time.Time
}

// RequestPlayback は動画を再生するための期限付きURLを出す。
//
// 利用者には「再生できる状態」で「付いている投稿・メッセージを見てよい」ときだけ出す。
// URL の期限は保存期間の期限を越えない。期限が過ぎた後は、Azure のライフサイクル管理が
// まだ実体を消していなくても、新しいURLは出ず、出したURLも期限で切れる。
func (s *Service) RequestPlayback(ctx context.Context, mediaID int64) (Playback, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return Playback{}, err
	}
	p, err := s.Policy(ctx)
	if err != nil {
		return Playback{}, disabledErr(err)
	}
	v, parent, err := s.d.Videos.GetVideo(ctx, mediaID)
	if err != nil {
		return Playback{}, err
	}
	notFound := apperr.NotFound("動画が見つかりません")
	if v == nil || parent == nil {
		return Playback{}, notFound
	}
	now := s.d.Now()
	expiry := now.Add(playbackURLTTL)

	if isAdmin(ctx) {
		// 管理者は通報の確認のため、外したもの・期限が過ぎたものも実体がある限り見られる。
		if v.PurgedAt != nil {
			return Playback{}, apperr.NotFound("動画のファイルは既に削除されています")
		}
	} else {
		allowed, err := s.canView(ctx, *parent)
		if err != nil {
			return Playback{}, err
		}
		if !allowed {
			// 見られない投稿・メッセージの動画は、あるかどうかも答えない。
			return Playback{}, notFound
		}
		switch v.Status(now, p.Retention()) {
		case model.VideoExpired:
			return Playback{}, apperr.NotFound(fmt.Sprintf("動画の保存期間（%d日間）が終了しました。", p.RetentionDays))
		case model.VideoRemoved:
			return Playback{}, apperr.NotFound("この動画は削除されました")
		}
		if exp := v.ExpiresAt(p.Retention()); exp.Before(expiry) {
			expiry = exp
		}
	}

	url, err := s.d.Store.SignedReadURL(ctx, v.StorageKey, expiry)
	if err != nil {
		return Playback{}, fmt.Errorf("failed to issue a playback URL: %w", err)
	}
	s.addStats(ctx, repository.VideoDailyStats{PlaybackRequests: 1, PlaybackBytes: v.SizeBytes})
	return Playback{URL: url, ExpiresAt: expiry}, nil
}

func (s *Service) canView(ctx context.Context, parent model.VideoParent) (bool, error) {
	switch parent.Kind {
	case model.VideoParentPost:
		if s.d.CanViewPost == nil {
			return false, nil
		}
		return s.d.CanViewPost(ctx, parent.ID)
	case model.VideoParentMessage:
		if s.d.CanViewMessage == nil {
			return false, nil
		}
		return s.d.CanViewMessage(ctx, parent.ID)
	}
	return false, nil
}
