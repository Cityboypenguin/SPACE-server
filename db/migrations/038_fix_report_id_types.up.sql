ALTER TABLE user_reports
    MODIFY reporter_id BIGINT NOT NULL,
    DROP INDEX idx_user_reports_status,
    DROP INDEX idx_user_reports_created_at,
    ADD INDEX idx_user_reports_status_created (status, created_at);
