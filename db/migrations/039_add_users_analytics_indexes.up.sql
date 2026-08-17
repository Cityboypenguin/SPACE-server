ALTER TABLE users
    ADD INDEX idx_users_created_at (created_at),
    ADD INDEX idx_users_last_active_at (last_active_at),
    ADD INDEX idx_users_status (status);
