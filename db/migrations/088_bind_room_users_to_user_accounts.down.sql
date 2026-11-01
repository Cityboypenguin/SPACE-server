ALTER TABLE room_users
    DROP FOREIGN KEY fk_room_users_account,
    ADD CONSTRAINT room_users_ibfk_2 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
