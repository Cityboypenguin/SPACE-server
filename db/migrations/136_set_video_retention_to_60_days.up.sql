-- 動画の保存期間を 60 日にする（2026-11-20 の決定。133 の初期値 90 から変える）。
-- 管理画面で既に変えた値は上書きしない（90 のままの行だけ変える）。
UPDATE system_settings SET `value` = '60', `updated_at` = UNIX_TIMESTAMP()
WHERE `key` = 'video_retention_days' AND `value` = '90';
