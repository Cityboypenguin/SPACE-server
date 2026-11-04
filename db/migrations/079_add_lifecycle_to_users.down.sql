ALTER TABLE users
    ADD INDEX idx_users_status (status),
    DROP INDEX idx_users_status_deactivated_at,
    DROP COLUMN deleted_at,
    DROP COLUMN deactivated_at;
