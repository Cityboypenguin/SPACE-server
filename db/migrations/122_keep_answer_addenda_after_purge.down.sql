ALTER TABLE answer_addenda
    DROP FOREIGN KEY fk_answer_addenda_retained,
    ADD CONSTRAINT fk_answer_addenda_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
