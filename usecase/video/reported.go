package video

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// 通報と動画。
//
// 通報の写し（model.ReportedContent）は動画を写し取らず、元のキーを持つ。ファイルは
// 次のように通報への対応が終わるまで残る。
//   - 通常: 未対応・確認中の通報が付いている投稿・メッセージの動画は日次の削除から外す
//     （repository.VideoPurger）。
//   - 退会: media の行は消えるが、ファイルは消さずに reported_video_retentions に記録する
//     （RetainReportedVideos）。
// 対応が終わったら（解決・却下）、通常の動画は期限・外した・削除の決まりで、退会の分は
// 次の日次の処理で消える（purgeRetentions）。

// SnapshotVideo は通報の写しに入れる、メッセージの動画。付いていなければ nil。
// 外した・期限が過ぎた・実体を消した動画も写す（対応する側が「何が付いていたか」を知るため）。
func (s *Service) SnapshotVideo(ctx context.Context, messageID int64) (*model.ReportedContentVideo, error) {
	videos, err := s.d.Videos.ListVideosByMessageIDs(ctx, []int64{messageID})
	if err != nil {
		return nil, err
	}
	v := videos[messageID]
	if v == nil {
		return nil, nil
	}
	return &model.ReportedContentVideo{
		MediaID:      v.MediaID,
		StorageKey:   v.StorageKey,
		ThumbnailKey: v.ThumbnailKey,
		SizeBytes:    v.SizeBytes,
		DurationMs:   v.DurationMs,
		Width:        v.Width,
		Height:       v.Height,
		HasAudio:     v.HasAudio,
		CreatedAt:    v.CreatedAt,
	}, nil
}

// RetainReportedVideos は退会のトランザクションの中で、通報中のメッセージの動画を残す記録を作り、
// 消してはいけないファイルのキーを返す。呼び出し側はこれらを消す対象から外す。
func (s *Service) RetainReportedVideos(ctx context.Context, userID int64) ([]string, error) {
	return s.d.Videos.RetainReportedMessageVideos(ctx, userID, s.d.Now())
}

// ReportedVideoView は通報の写しの動画の、管理者向けの表示。
type ReportedVideoView struct {
	Video *model.ReportedContentVideo
	// Available はファイルがまだ残っていて再生できるか。
	Available    bool
	ThumbnailURL string
}

// fileRemains は通報の写しが指す動画のファイルが残っているか。
func (s *Service) fileRemains(ctx context.Context, mediaID int64) (bool, error) {
	v, _, err := s.d.Videos.GetVideo(ctx, mediaID)
	if err != nil {
		return false, err
	}
	if v != nil {
		return v.PurgedAt == nil, nil
	}
	rv, err := s.d.Videos.GetRetention(ctx, mediaID)
	if err != nil {
		return false, err
	}
	return rv != nil && rv.PurgedAt == nil, nil
}

// ViewReportedVideo は管理者だけが呼ぶ（通報の写しは管理者しか読めない）。
func (s *Service) ViewReportedVideo(ctx context.Context, rv *model.ReportedContentVideo) (ReportedVideoView, error) {
	view := ReportedVideoView{Video: rv}
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return view, err
	}
	ok, err := s.fileRemains(ctx, rv.MediaID)
	if err != nil || !ok {
		return view, err
	}
	view.Available = true
	expiry := s.d.Now().Add(thumbnailURLTTL).Truncate(thumbnailURLWindow).Add(thumbnailURLWindow)
	url, err := s.d.Store.SignedReadURL(ctx, rv.ThumbnailKey, expiry)
	if err != nil {
		logger.Log.Error().Err(err).Str("component", "video").Int64("media_id", rv.MediaID).Msg("failed to sign a reported video thumbnail URL")
		return view, nil
	}
	view.ThumbnailURL = url
	return view, nil
}

// RequestReportedPlayback は通報の写しの動画を管理者が再生するためのURLを出す。
// 退会で media の行が消えた動画も、通報への対応が終わるまではここから再生できる。
func (s *Service) RequestReportedPlayback(ctx context.Context, rv *model.ReportedContentVideo) (Playback, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return Playback{}, err
	}
	if rv == nil {
		return Playback{}, apperr.NotFound("この通報に動画はありません")
	}
	ok, err := s.fileRemains(ctx, rv.MediaID)
	if err != nil {
		return Playback{}, err
	}
	if !ok {
		return Playback{}, apperr.NotFound("動画のファイルは既に削除されています")
	}
	expiry := s.d.Now().Add(playbackURLTTL)
	url, err := s.d.Store.SignedReadURL(ctx, rv.StorageKey, expiry)
	if err != nil {
		return Playback{}, fmt.Errorf("failed to issue a playback URL: %w", err)
	}
	return Playback{URL: url, ExpiresAt: expiry}, nil
}
