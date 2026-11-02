-- 追記は質問の本文と同じく会話の一部なので、退会後も残す（users を参照する）。
ALTER TABLE question_addenda
    DROP FOREIGN KEY fk_question_addenda_user,
    ADD CONSTRAINT fk_question_addenda_retained FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
