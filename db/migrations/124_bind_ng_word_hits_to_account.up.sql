-- NGワードで止めた記録は、その人に結び付く情報なので個人情報と一緒に消す。
ALTER TABLE ng_word_hits
    DROP FOREIGN KEY fk_ng_word_hits_user,
    ADD CONSTRAINT fk_ng_word_hits_account FOREIGN KEY (user_id) REFERENCES user_accounts(user_id) ON DELETE CASCADE;
