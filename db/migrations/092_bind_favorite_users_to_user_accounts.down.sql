ALTER TABLE favorite_users
    DROP FOREIGN KEY fk_favorite_users_account,
    DROP FOREIGN KEY fk_favorite_users_favorite_account,
    ADD CONSTRAINT favorite_users_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT favorite_users_ibfk_2 FOREIGN KEY (favorite_user_id) REFERENCES users(id) ON DELETE CASCADE;
