-- 退会で消える動画のうち、未対応・確認中の通報が付いたメッセージの動画を残した記録。
--
-- 退会では media の行が消え、video_assets も CASCADE で消える。通報の写し
-- （user_reports.message_snapshot）は動画のキーを持っているので、ファイルを消さずに
-- 残し、ここで後片付けを追う。通報への対応が終わったら日次の処理がファイルを消して
-- purged_at を入れる。行は通報の写しから「削除済み」を出すために残す。
--
-- media_id は元の media の行の ID（行はもう無いので外部キーは張らない）。
CREATE TABLE reported_video_retentions (
    media_id              BIGINT       NOT NULL,
    storage_key           VARCHAR(255) NOT NULL,
    thumbnail_key         VARCHAR(255) NOT NULL,
    size_bytes            BIGINT       NOT NULL,
    message_id            BIGINT       NOT NULL,
    created_at            BIGINT       NOT NULL,
    purged_at             BIGINT       NULL,
    purge_attempts        INT          NOT NULL DEFAULT 0,
    purge_next_attempt_at BIGINT       NULL,
    purge_last_error      VARCHAR(255) NULL,
    PRIMARY KEY (media_id),
    INDEX idx_reported_video_retentions_purge (purged_at, purge_next_attempt_at)
);
