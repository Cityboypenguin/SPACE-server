ALTER TABLE messages
    ADD COLUMN thread_room_id BIGINT NULL,
    ADD CONSTRAINT fk_messages_thread_room FOREIGN KEY (thread_room_id) REFERENCES rooms(id) ON DELETE SET NULL;
