package video

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// purgeRetryDelay は実体の削除に失敗した動画を、次に試すまでの間隔。
// 日次の処理は失敗があると1時間後にもう一度走る（internal/dailyjob）ので、それに合わせる。
const purgeRetryDelay = time.Hour

// errPurgeIncomplete は1本以上消せなかったこと。dailyjob はこれを見て1時間後にやり直す。
var errPurgeIncomplete = errors.New("some videos could not be purged; will retry")

// PurgeDue は実体を消してよい動画の、動画ファイルとサムネイルを消す。消した本数を返す。
//
// 対象は次のどれか（いずれも未対応・確認中の通報が付いているものは除く）。
//   - 登録から「保存期間 + 猶予」を過ぎた
//   - 投稿者・管理者が外してから猶予を過ぎた
//   - 付いている投稿・メッセージが削除されてから猶予を過ぎた
//
// 利用者からはこれより前（期限の時点、外した時点）に見えなくなっている。猶予は、
// 保存期間を誤って短くした・誤って外した、といった操作を戻せるようにするためのもの。
//
// 1回に消す本数は設定（video_purge_batch_size）で抑える。保存期間を短くして大量に
// 対象になっても、一度に消し切らず日をまたいで進める。
//
// 消せなかった動画は記録して先へ進み、最後にエラーを返す（1時間後にやり直される）。
// 動画1本の失敗で残り全部が止まらないようにするため。
func (s *Service) PurgeDue(ctx context.Context, now time.Time) (int, error) {
	p, err := LoadPolicy(ctx, s.d.Settings)
	if err != nil {
		// 設定が無いまま既定値で消していくことはしない。
		return 0, err
	}
	candidates, err := s.d.Videos.ListVideosToPurge(ctx, repository.VideoPurgeQuery{
		Now:           now,
		ExpiredBefore: now.Add(-p.Retention() - p.PurgeGrace()),
		RemovedBefore: now.Add(-p.PurgeGrace()),
		Limit:         p.PurgeBatchSize,
	})
	if err != nil {
		return 0, err
	}

	purged, failed := 0, 0
	for _, v := range candidates {
		if ctx.Err() != nil {
			break
		}
		if err := s.purgeOne(ctx, v.StorageKey, v.ThumbnailKey); err != nil {
			failed++
			logger.Log.Error().Err(err).Str("component", "video_purge").Int64("media_id", v.MediaID).
				Msg("failed to delete a video; will retry")
			if recErr := s.d.Videos.RecordVideoPurgeFailure(ctx, v.MediaID, err.Error(), now.Add(purgeRetryDelay)); recErr != nil {
				logger.Log.Error().Err(recErr).Str("component", "video_purge").Int64("media_id", v.MediaID).
					Msg("failed to record a video purge failure")
			}
			continue
		}
		if err := s.d.Videos.MarkVideoPurged(ctx, v.MediaID, now); err != nil {
			// 実体は消えている。次回また対象になるが、消す操作は無いものに対しても成功するので害は無い。
			failed++
			logger.Log.Error().Err(err).Str("component", "video_purge").Int64("media_id", v.MediaID).
				Msg("deleted a video but failed to record it")
			continue
		}
		purged++
	}
	// 退会のあと通報のために残していた動画は、対応が終わったものから消す。
	if ctx.Err() == nil {
		rp, rf := s.purgeRetentions(ctx, now, p.PurgeBatchSize)
		purged += rp
		failed += rf
	}

	if purged > 0 || failed > 0 {
		s.addStats(ctx, repository.VideoDailyStats{Purged: int64(purged), PurgeFailures: int64(failed)})
	}
	if failed > 0 {
		return purged, errPurgeIncomplete
	}
	return purged, ctx.Err()
}

// purgeOne は動画とサムネイルを消す。無いものを消すのは成功として扱われる（置き場の実装）。
func (s *Service) purgeOne(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if err := s.d.Store.DeleteObject(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// purgeRetentions は退会のあと通報のために残していた動画のうち、通報への対応が終わったものを消す。
// 対応が終わった時点で猶予は置かない（期限・外した時点からの猶予は退会の前に過ぎているか、
// 投稿者がもういないので戻す操作が無い）。
func (s *Service) purgeRetentions(ctx context.Context, now time.Time, limit int) (purged, failed int) {
	items, err := s.d.Videos.ListRetentionsToPurge(ctx, now, limit)
	if err != nil {
		logger.Log.Error().Err(err).Str("component", "video_purge").Msg("failed to list retained reported videos")
		return 0, 1
	}
	for _, rv := range items {
		if ctx.Err() != nil {
			break
		}
		if err := s.purgeOne(ctx, rv.StorageKey, rv.ThumbnailKey); err != nil {
			failed++
			logger.Log.Error().Err(err).Str("component", "video_purge").Int64("media_id", rv.MediaID).
				Msg("failed to delete a retained reported video; will retry")
			if recErr := s.d.Videos.RecordRetentionPurgeFailure(ctx, rv.MediaID, err.Error(), now.Add(purgeRetryDelay)); recErr != nil {
				logger.Log.Error().Err(recErr).Str("component", "video_purge").Int64("media_id", rv.MediaID).
					Msg("failed to record a retained video purge failure")
			}
			continue
		}
		if err := s.d.Videos.MarkRetentionPurged(ctx, rv.MediaID, now); err != nil {
			failed++
			logger.Log.Error().Err(err).Str("component", "video_purge").Int64("media_id", rv.MediaID).
				Msg("deleted a retained video but failed to record it")
			continue
		}
		purged++
	}
	return purged, failed
}
