package graph

import (
	"context"
	"fmt"
	"strconv"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/upload"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	videousecase "github.com/Cityboypenguin/SPACE-server/usecase/video"
)

const videoIDKind = "video"

// videoField は Post.video / Message.video の共通の本体。
//
// 投稿者が外した動画は返さない（投稿は動画の無いものとして見せる）。管理者が外した
// もの・投稿ごと削除されたものは REMOVED、期限が過ぎたものは EXPIRED として返し、
// 表示側が「保存期間が終了しました」などを出し分ける。
func (r *Resolver) videoField(ctx context.Context, v *model.Video) (*gqlmodel.Video, error) {
	if v == nil || r.VideoService == nil {
		return nil, nil
	}
	if v.RemovedReason != nil && *v.RemovedReason == model.VideoRemovedByAuthor {
		return nil, nil
	}
	view, err := r.VideoService.ViewOf(ctx, v)
	if err != nil {
		return nil, err
	}
	return toGraphVideo(view), nil
}

func toGraphVideo(view videousecase.View) *gqlmodel.Video {
	v := view.Video
	out := &gqlmodel.Video{
		ID:            encodeGraphID(videoIDKind, v.MediaID),
		Status:        gqlmodel.VideoStatus(view.Status),
		DurationMs:    int32(v.DurationMs),
		Width:         int32(v.Width),
		Height:        int32(v.Height),
		SizeBytes:     float64(v.SizeBytes),
		HasAudio:      v.HasAudio,
		ExpiresAt:     view.ExpiresAt.UTC().Format(time.RFC3339),
		RetentionDays: int32(view.RetentionDays),
		CreatedAt:     v.CreatedAt.UTC().Format(time.RFC3339),
	}
	if view.ThumbnailURL != "" {
		u := view.ThumbnailURL
		out.ThumbnailURL = &u
	}
	return out
}

func (r *Resolver) videoSettings(ctx context.Context) (*gqlmodel.VideoSettings, error) {
	if r.VideoService == nil {
		return nil, fmt.Errorf("video is not configured")
	}
	p, err := r.VideoService.Policy(ctx)
	if err != nil {
		return nil, err
	}
	return toGraphVideoSettings(p, r.VideoService.BackstopDays()), nil
}

func toGraphVideoSettings(p videousecase.Policy, backstopDays int) *gqlmodel.VideoSettings {
	return &gqlmodel.VideoSettings{
		RetentionDays:           int32(p.RetentionDays),
		PurgeGraceDays:          int32(p.PurgeGraceDays),
		PurgeBatchSize:          int32(p.PurgeBatchSize),
		DailyUploadLimitPerUser: float64(p.DailyUploadLimit),
		DailyBytesLimitPerUser:  float64(p.DailyBytesLimit),
		TotalBytesLimit:         float64(p.TotalBytesLimit),
		MaxRetentionDays:        int32(videousecase.MaxRetentionDays(backstopDays, p.PurgeGraceDays)),
		LifecycleBackstopDays:   int32(backstopDays),
		MaxFileBytes:            float64(upload.VideoMaxBytes),
		MaxDurationSeconds:      60,
	}
}

func toGraphVideoUsage(u repository.VideoUsage) *gqlmodel.VideoUsage {
	return &gqlmodel.VideoUsage{Count: float64(u.Count), Bytes: float64(u.Bytes)}
}

func toGraphVideoDailyStat(d repository.VideoDailyStats) *gqlmodel.VideoDailyStat {
	day := ""
	if d.Day > 0 {
		s := strconv.Itoa(d.Day)
		if len(s) == 8 {
			day = s[0:4] + "-" + s[4:6] + "-" + s[6:8]
		}
	}
	return &gqlmodel.VideoDailyStat{
		Day:              day,
		Uploads:          float64(d.Uploads),
		UploadBytes:      float64(d.UploadBytes),
		UploadFailures:   float64(d.UploadFailures),
		PlaybackRequests: float64(d.PlaybackRequests),
		PlaybackBytes:    float64(d.PlaybackBytes),
		Purged:           float64(d.Purged),
		PurgeFailures:    float64(d.PurgeFailures),
	}
}

func toGraphRetentionPreview(p videousecase.RetentionPreview) *gqlmodel.VideoRetentionPreview {
	return &gqlmodel.VideoRetentionPreview{
		CurrentDays:        int32(p.CurrentDays),
		NewDays:            int32(p.NewDays),
		MaxDays:            int32(p.MaxDays),
		NewlyExpired:       toGraphVideoUsage(p.NewlyExpired),
		ExpiringWithinWeek: toGraphVideoUsage(p.ExpiringWithinWeek),
	}
}

// toGraphReportedVideo は通報の写しの動画。ファイルが残っているかとサムネイルは
// ReportedVideo のリゾルバが選ばれたときだけ調べる。
func toGraphReportedVideo(v *model.ReportedContentVideo) *gqlmodel.ReportedVideo {
	if v == nil {
		return nil
	}
	return &gqlmodel.ReportedVideo{
		ID:         encodeGraphID(videoIDKind, v.MediaID),
		DurationMs: int32(v.DurationMs),
		Width:      int32(v.Width),
		Height:     int32(v.Height),
		HasAudio:   v.HasAudio,
		SizeBytes:  float64(v.SizeBytes),
		CreatedAt:  v.CreatedAt.UTC().Format(time.RFC3339),
		Source:     v,
	}
}

// reportedVideoView は ReportedVideo のリゾルバが共有する。動画が無い構成なら空。
func (r *Resolver) reportedVideoView(ctx context.Context, v *gqlmodel.ReportedVideo) (videousecase.ReportedVideoView, error) {
	if r.VideoService == nil || v == nil || v.Source == nil {
		return videousecase.ReportedVideoView{}, nil
	}
	return r.VideoService.ViewReportedVideo(ctx, v.Source)
}
