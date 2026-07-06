ALTER TABLE users
    ADD INDEX idx_users_created_at (created_at),
    ADD INDEX idx_users_last_active_at (last_active_at),
    ADD INDEX idx_users_status (status);

ALTER TABLE posts
    ADD INDEX idx_posts_type_deleted_created (parent_id, deleted_at, created_at),
    ADD INDEX idx_posts_user_deleted (user_id, deleted_at);

ALTER TABLE messages
    ADD INDEX idx_messages_created_at (created_at),
    ADD INDEX idx_messages_room_created (room_id, created_at);

ALTER TABLE favorites
    ADD INDEX idx_favorites_created_at (created_at);
