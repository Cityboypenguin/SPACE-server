ALTER TABLE polls
    DROP FOREIGN KEY fk_polls_author_user,
    ADD CONSTRAINT fk_polls_author FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE;
