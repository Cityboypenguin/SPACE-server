-- questions は退会後も残す（投稿者は「削除されたアカウント」と表示する）。
-- users の行は消さない前提なので CASCADE を RESTRICT に替え、誤って消そうとしたら DB が止める。
ALTER TABLE questions
    DROP FOREIGN KEY fk_questions_asker,
    ADD CONSTRAINT fk_questions_asker_user FOREIGN KEY (asker_user_id) REFERENCES users(id) ON DELETE RESTRICT;
