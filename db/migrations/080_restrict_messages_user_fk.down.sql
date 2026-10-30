ALTER TABLE messages
    DROP FOREIGN KEY fk_messages_user,
    ADD CONSTRAINT messages_ibfk_2 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
