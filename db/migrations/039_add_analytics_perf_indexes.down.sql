ALTER TABLE users
    DROP INDEX idx_users_created_at,
    DROP INDEX idx_users_last_active_at,
    DROP INDEX idx_users_status;

ALTER TABLE posts
    DROP INDEX idx_posts_type_deleted_created,
    DROP INDEX idx_posts_user_deleted;

ALTER TABLE messages
    DROP INDEX idx_messages_created_at,
    DROP INDEX idx_messages_room_created;

ALTER TABLE favorites
    DROP INDEX idx_favorites_created_at;
