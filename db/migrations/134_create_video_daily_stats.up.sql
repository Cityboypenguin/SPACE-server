-- 動画の量の日ごとの集計（管理画面の adminVideoStorageStats）。day は JST の yyyymmdd。
--
-- playback_bytes は再生用 URL を出した回数 × 動画の大きさで、実際の転送量ではない
-- （途中で止めた・シークした・ブラウザーのキャッシュから出た分を区別できない）。
-- 実際の転送量は Azure Monitor のストレージアカウントの Egress で見る。
CREATE TABLE video_daily_stats (
    day               INT    NOT NULL,
    uploads           INT    NOT NULL DEFAULT 0,
    upload_bytes      BIGINT NOT NULL DEFAULT 0,
    upload_failures   INT    NOT NULL DEFAULT 0,
    playback_requests INT    NOT NULL DEFAULT 0,
    playback_bytes    BIGINT NOT NULL DEFAULT 0,
    purged            INT    NOT NULL DEFAULT 0,
    purge_failures    INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (day)
);
