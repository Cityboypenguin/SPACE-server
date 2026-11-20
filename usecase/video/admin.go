package video

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// RetentionPreview は保存期間を変えたときの影響。
type RetentionPreview struct {
	CurrentDays int
	NewDays     int
	// MaxDays は設定できる上限（Azure の安全網との兼ね合い。MaxRetentionDays 参照）。
	MaxDays int
	// NewlyExpired は変更した瞬間に見えなくなる動画（今は見えている）。
	NewlyExpired repository.VideoUsage
	// ExpiringWithinWeek は変更後7日以内に期限が来る動画（NewlyExpired を除く）。
	ExpiringWithinWeek repository.VideoUsage
}

// PreviewRetention は保存期間を newDays にしたときの影響を返す。何も変えない。
func (s *Service) PreviewRetention(ctx context.Context, newDays int) (RetentionPreview, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return RetentionPreview{}, err
	}
	p, err := LoadPolicy(ctx, s.d.Settings)
	if err != nil {
		return RetentionPreview{}, disabledErr(err)
	}
	maxDays := MaxRetentionDays(s.d.BackstopDays, p.PurgeGraceDays)
	if newDays < 1 || newDays > maxDays {
		return RetentionPreview{}, apperr.InvalidInput(fmt.Sprintf("保存期間は1〜%d日で指定してください", maxDays))
	}
	now := s.d.Now()
	cutoff := func(days int) time.Time { return now.Add(-time.Duration(days) * day) }

	// 「期限が来ている」＝登録が now - 保存期間 より前。今の設定で既に来ているものを引いて、
	// 新しい設定で初めて来るものだけを数える。
	expiredNow, err := s.d.Videos.VideosExpiringBefore(ctx, cutoff(p.RetentionDays))
	if err != nil {
		return RetentionPreview{}, err
	}
	expiredNew, err := s.d.Videos.VideosExpiringBefore(ctx, cutoff(newDays))
	if err != nil {
		return RetentionPreview{}, err
	}
	expiredWeek, err := s.d.Videos.VideosExpiringBefore(ctx, cutoff(newDays).Add(7*day))
	if err != nil {
		return RetentionPreview{}, err
	}
	newly := sub(expiredNew, expiredNow)
	return RetentionPreview{
		CurrentDays:        p.RetentionDays,
		NewDays:            newDays,
		MaxDays:            maxDays,
		NewlyExpired:       newly,
		ExpiringWithinWeek: sub(sub(expiredWeek, expiredNow), newly),
	}, nil
}

func sub(a, b repository.VideoUsage) repository.VideoUsage {
	return repository.VideoUsage{Count: max(a.Count-b.Count, 0), Bytes: max(a.Bytes-b.Bytes, 0)}
}

// UpdateRetention は保存期間を変える。
//
// expectedNewlyExpired には、直前の PreviewRetention で確認した NewlyExpired.Count を
// 渡す。確認してから変更するまでの間に件数が変わっていたら（時間が経って期限の来る
// 動画が増えた、など）変更しない。確認した内容と違う結果を、確認したつもりで
// 起こさないため。
func (s *Service) UpdateRetention(ctx context.Context, newDays int, expectedNewlyExpired int64) (RetentionPreview, error) {
	preview, err := s.PreviewRetention(ctx, newDays)
	if err != nil {
		return RetentionPreview{}, err
	}
	if preview.NewlyExpired.Count != expectedNewlyExpired {
		return RetentionPreview{}, apperr.Conflict("影響する動画の本数が確認したときから変わりました。もう一度確認してください")
	}
	if err := s.d.Settings.Update(ctx, SettingRetentionDays, strconv.Itoa(newDays), s.d.Now().Unix()); err != nil {
		return RetentionPreview{}, err
	}
	s.invalidate()
	return preview, nil
}

// Limits は管理画面で変えられる、保存期間以外の設定。
type Limits struct {
	PurgeGraceDays   int
	PurgeBatchSize   int
	DailyUploadLimit int64
	DailyBytesLimit  int64
	TotalBytesLimit  int64
}

// UpdateLimits は保存期間以外の設定を変える。
func (s *Service) UpdateLimits(ctx context.Context, l Limits) (Policy, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return Policy{}, err
	}
	p, err := LoadPolicy(ctx, s.d.Settings)
	if err != nil {
		return Policy{}, disabledErr(err)
	}
	if l.PurgeGraceDays < 0 || l.PurgeBatchSize < 1 || l.DailyUploadLimit < 0 || l.DailyBytesLimit < 0 || l.TotalBytesLimit < 0 {
		return Policy{}, apperr.InvalidInput("設定の値が正しくありません")
	}
	if p.RetentionDays > MaxRetentionDays(s.d.BackstopDays, l.PurgeGraceDays) {
		return Policy{}, apperr.InvalidInput("この猶予では、保存期間と合わせて Azure の自動削除の日数を超えます")
	}
	now := s.d.Now().Unix()
	for key, value := range map[string]string{
		SettingPurgeGraceDays:   strconv.Itoa(l.PurgeGraceDays),
		SettingPurgeBatchSize:   strconv.Itoa(l.PurgeBatchSize),
		SettingDailyUploadLimit: strconv.FormatInt(l.DailyUploadLimit, 10),
		SettingDailyBytesLimit:  strconv.FormatInt(l.DailyBytesLimit, 10),
		SettingTotalBytesLimit:  strconv.FormatInt(l.TotalBytesLimit, 10),
	} {
		if err := s.d.Settings.Update(ctx, key, value, now); err != nil {
			return Policy{}, err
		}
	}
	s.invalidate()
	return LoadPolicy(ctx, s.d.Settings)
}

// AdminRemove は管理者が動画を見えなくする。実体は猶予の後に日次の処理で消える。
func (s *Service) AdminRemove(ctx context.Context, mediaID int64) error {
	claims, err := authz.RequireAdmin(ctx)
	if err != nil {
		return err
	}
	v, _, err := s.d.Videos.GetVideo(ctx, mediaID)
	if err != nil {
		return err
	}
	if v == nil {
		return apperr.NotFound("動画が見つかりません")
	}
	return s.d.Videos.MarkVideoRemoved(ctx, mediaID, model.VideoRemovedByAdmin, claims.ID, s.d.Now())
}

// Stats は管理画面に出す量の集計。
type Stats struct {
	Policy        Policy
	MaxDays       int
	BackstopDays  int
	Stored        repository.VideoUsage
	PurgeFailing  int64
	Today         repository.VideoDailyStats
	ThisMonth     repository.VideoDailyStats
	Daily         []repository.VideoDailyStats
	StatsFromDay  int
	StatsUntilDay int
}

// statsDays は日ごとの集計を返す日数。
const statsDays = 31

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return Stats{}, err
	}
	p, err := LoadPolicy(ctx, s.d.Settings)
	if err != nil {
		return Stats{}, disabledErr(err)
	}
	now := s.d.Now()
	stored, err := s.d.Videos.StoredVideoUsage(ctx)
	if err != nil {
		return Stats{}, err
	}
	failing, err := s.d.Videos.PurgeFailingCount(ctx)
	if err != nil {
		return Stats{}, err
	}
	local := now.In(jst)
	monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, jst)
	from := now.Add(-statsDays * day)
	if monthStart.Before(from) {
		from = monthStart
	}
	daily, err := s.d.Videos.ListVideoDailyStats(ctx, dayKey(from), dayKey(now))
	if err != nil {
		return Stats{}, err
	}
	out := Stats{
		Policy:        p,
		MaxDays:       MaxRetentionDays(s.d.BackstopDays, p.PurgeGraceDays),
		BackstopDays:  s.d.BackstopDays,
		Stored:        stored,
		PurgeFailing:  failing,
		StatsFromDay:  dayKey(from),
		StatsUntilDay: dayKey(now),
	}
	today, month := dayKey(now), dayKey(monthStart)
	for _, d := range daily {
		if d.Day == today {
			out.Today = d
		}
		if d.Day >= month {
			addInto(&out.ThisMonth, d)
		}
	}
	out.Daily = daily
	return out, nil
}

func addInto(dst *repository.VideoDailyStats, d repository.VideoDailyStats) {
	dst.Uploads += d.Uploads
	dst.UploadBytes += d.UploadBytes
	dst.UploadFailures += d.UploadFailures
	dst.PlaybackRequests += d.PlaybackRequests
	dst.PlaybackBytes += d.PlaybackBytes
	dst.Purged += d.Purged
	dst.PurgeFailures += d.PurgeFailures
}
