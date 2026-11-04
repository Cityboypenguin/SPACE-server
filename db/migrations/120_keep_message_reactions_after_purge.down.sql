ALTER TABLE message_reactions
    DROP FOREIGN KEY fk_message_reactions_retained,
    ADD CONSTRAINT fk_message_reactions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
