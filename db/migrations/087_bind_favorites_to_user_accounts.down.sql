ALTER TABLE favorites
    DROP FOREIGN KEY fk_favorites_account,
    ADD CONSTRAINT favorites_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
