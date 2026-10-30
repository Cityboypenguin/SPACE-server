ALTER TABLE message_mentions
    DROP FOREIGN KEY fk_message_mentions_account,
    ADD CONSTRAINT fk_message_mentions_user FOREIGN KEY (mentioned_user_id) REFERENCES users(id) ON DELETE CASCADE;
