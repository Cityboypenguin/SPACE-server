ALTER TABLE room_users
    ADD COLUMN welcome_message_id BIGINT NULL,
    ADD COLUMN greeting_sent BOOLEAN NOT NULL DEFAULT FALSE;
