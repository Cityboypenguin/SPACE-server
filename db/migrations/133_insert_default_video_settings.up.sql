-- 動画の設定の初期値。値はここ（DB）でだけ持ち、コードには書かない。
-- 行が無いと動画の投稿・再生は止まる（usecase/video の Policy 参照）。
--
--   video_retention_days               保存期間（日）。投稿成立（動画の登録）から数える
--   video_purge_grace_days             期限が来て見えなくしてから実体を消すまでの猶予（日）
--   video_purge_batch_size             日次の削除で1回に消す本数の上限
--   video_daily_upload_limit_per_user  1人1日の投稿本数の上限（0 は無制限）
--   video_daily_bytes_limit_per_user   1人1日の投稿容量の上限（バイト、0 は無制限）
--   video_total_bytes_limit            保存中の動画の合計容量の上限（バイト、0 は無制限）
INSERT INTO system_settings (`key`, `value`, `updated_at`) VALUES
    ('video_retention_days', '90', UNIX_TIMESTAMP()),
    ('video_purge_grace_days', '3', UNIX_TIMESTAMP()),
    ('video_purge_batch_size', '200', UNIX_TIMESTAMP()),
    ('video_daily_upload_limit_per_user', '0', UNIX_TIMESTAMP()),
    ('video_daily_bytes_limit_per_user', '0', UNIX_TIMESTAMP()),
    ('video_total_bytes_limit', '0', UNIX_TIMESTAMP());
