ALTER TABLE users
    DROP INDEX idx_users_created_at,
    DROP INDEX idx_users_last_active_at,
    DROP INDEX idx_users_status;
