ALTER TABLE user_reports
    ADD INDEX idx_user_reports_status (status),
    ADD INDEX idx_user_reports_target_type (target_type),
    ADD INDEX idx_user_reports_reporter_id (reporter_id),
    ADD INDEX idx_user_reports_created_at (created_at);
