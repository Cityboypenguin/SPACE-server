-- リアクションは answer_likes と同じく会話の一部なので、退会後も残す（users を参照する）。
ALTER TABLE message_reactions
    DROP FOREIGN KEY fk_message_reactions_user,
    ADD CONSTRAINT fk_message_reactions_retained FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
