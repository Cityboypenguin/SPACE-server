ALTER TABLE post_mentions
    DROP FOREIGN KEY fk_post_mentions_account,
    ADD CONSTRAINT fk_post_mentions_user FOREIGN KEY (mentioned_user_id) REFERENCES users(id) ON DELETE CASCADE;
