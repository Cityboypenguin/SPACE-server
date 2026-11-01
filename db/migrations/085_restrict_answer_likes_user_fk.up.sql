-- answer_likes は退会後も残す（投稿者は「削除されたアカウント」と表示する）。
-- users の行は消さない前提なので CASCADE を RESTRICT に替え、誤って消そうとしたら DB が止める。
ALTER TABLE answer_likes
    DROP FOREIGN KEY fk_answer_likes_user,
    ADD CONSTRAINT fk_answer_likes_liker FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
