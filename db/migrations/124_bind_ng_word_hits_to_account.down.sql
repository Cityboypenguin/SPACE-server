ALTER TABLE ng_word_hits
    DROP FOREIGN KEY fk_ng_word_hits_account,
    ADD CONSTRAINT fk_ng_word_hits_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
