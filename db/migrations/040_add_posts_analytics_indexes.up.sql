ALTER TABLE posts
    ADD INDEX idx_posts_type_deleted_created (parent_id, deleted_at, created_at),
    ADD INDEX idx_posts_user_deleted (user_id, deleted_at);
