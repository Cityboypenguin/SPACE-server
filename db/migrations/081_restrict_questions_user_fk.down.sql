ALTER TABLE questions
    DROP FOREIGN KEY fk_questions_asker_user,
    ADD CONSTRAINT fk_questions_asker FOREIGN KEY (asker_user_id) REFERENCES users(id) ON DELETE CASCADE;
