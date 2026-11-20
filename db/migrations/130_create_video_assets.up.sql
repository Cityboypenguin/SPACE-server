-- 投稿・メッセージに付いた動画の実測値と後片付けの状態。media の行と1対1で、
-- 動画のファイル（media.storage_key）とサムネイル（thumbnail_key）は動画用の
-- コンテナーにある。
--
-- 保存期限の列は持たない。期限は media.created_at と system_settings の
-- video_retention_days から毎回計算する（保存期間を変えたら、まだ消していない既存の
-- 動画にもそのまま効かせるため）。
--
-- removed_at は期限より前に見えなくした時刻（投稿者が外した・管理者が外した）。
-- purged_at は実体を消し終えた時刻。行は消さずに残す（投稿の表示で「保存期間が
-- 終了しました」を出すため、および量の集計のため）。
-- purge_* は実体の削除に失敗したときのやり直しの記録。
CREATE TABLE video_assets (
    media_id              BIGINT       NOT NULL,
    thumbnail_key         VARCHAR(255) NOT NULL,
    size_bytes            BIGINT       NOT NULL,
    duration_ms           INT          NOT NULL,
    fps_milli             INT          NOT NULL,
    has_audio             TINYINT(1)   NOT NULL,
    removed_at            BIGINT       NULL,
    removed_reason        VARCHAR(20)  NULL,
    removed_by            BIGINT       NULL,
    purged_at             BIGINT       NULL,
    purge_attempts        INT          NOT NULL DEFAULT 0,
    purge_next_attempt_at BIGINT       NULL,
    purge_last_error      VARCHAR(255) NULL,
    PRIMARY KEY (media_id),
    INDEX idx_video_assets_purge (purged_at, purge_next_attempt_at),
    CONSTRAINT fk_video_assets_media FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);
