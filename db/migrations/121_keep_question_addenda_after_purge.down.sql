ALTER TABLE question_addenda
    DROP FOREIGN KEY fk_question_addenda_retained,
    ADD CONSTRAINT fk_question_addenda_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
