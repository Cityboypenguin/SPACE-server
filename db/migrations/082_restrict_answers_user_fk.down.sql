ALTER TABLE answers
    DROP FOREIGN KEY fk_answers_author_user,
    ADD CONSTRAINT fk_answers_author FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE;
