-- poll_votes は退会後も残す（投稿者は「削除されたアカウント」と表示する）。
-- users の行は消さない前提なので CASCADE を RESTRICT に替え、誤って消そうとしたら DB が止める。
ALTER TABLE poll_votes
    DROP FOREIGN KEY fk_poll_votes_user,
    ADD CONSTRAINT fk_poll_votes_voter FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
