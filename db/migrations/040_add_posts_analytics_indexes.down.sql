ALTER TABLE posts
    DROP INDEX idx_posts_type_deleted_created,
    DROP INDEX idx_posts_user_deleted;
