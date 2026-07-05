ALTER TABLE user_reports
    MODIFY reporter_id VARCHAR(255) NOT NULL,
    DROP INDEX idx_user_reports_status_created,
    ADD INDEX idx_user_reports_status (status),
    ADD INDEX idx_user_reports_created_at (created_at);
