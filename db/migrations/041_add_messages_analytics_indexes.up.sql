ALTER TABLE messages
    ADD INDEX idx_messages_created_at (created_at),
    ADD INDEX idx_messages_room_created (room_id, created_at);
