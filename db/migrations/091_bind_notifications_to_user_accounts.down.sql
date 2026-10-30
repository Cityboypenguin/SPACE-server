ALTER TABLE notifications
    DROP FOREIGN KEY fk_notifications_account,
    ADD CONSTRAINT notifications_ibfk_1 FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
