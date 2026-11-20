UPDATE system_settings SET `value` = '90', `updated_at` = UNIX_TIMESTAMP()
WHERE `key` = 'video_retention_days' AND `value` = '60';
