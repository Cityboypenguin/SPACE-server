ALTER TABLE user_session_summaries
    DROP FOREIGN KEY fk_user_session_summaries_account,
    ADD CONSTRAINT user_session_summaries_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
