INSERT INTO system_settings (`key`, `value`, `updated_at`)
VALUES ('timetable_enabled', 'true', UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `key` = `key`;