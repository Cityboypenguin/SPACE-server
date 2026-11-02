-- 追記は回答の本文と同じく会話の一部なので、退会後も残す（users を参照する）。
ALTER TABLE answer_addenda
    DROP FOREIGN KEY fk_answer_addenda_user,
    ADD CONSTRAINT fk_answer_addenda_retained FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
