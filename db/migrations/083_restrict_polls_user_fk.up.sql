-- polls は退会後も残す（投稿者は「削除されたアカウント」と表示する）。
-- users の行は消さない前提なので CASCADE を RESTRICT に替え、誤って消そうとしたら DB が止める。
ALTER TABLE polls
    DROP FOREIGN KEY fk_polls_author,
    ADD CONSTRAINT fk_polls_author_user FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE RESTRICT;
