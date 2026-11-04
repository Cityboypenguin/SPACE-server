ALTER TABLE posts
    DROP FOREIGN KEY fk_posts_account,
    ADD CONSTRAINT posts_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
